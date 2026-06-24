package services

import (
	"context"
	"encoding/json"
	"fmt"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"prophet-trader/models"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type CreateSuggestionRequest struct {
	Symbol     string   `json:"symbol"`
	Side       string   `json:"side"`
	ActionType string   `json:"action_type"`
	Quantity   float64  `json:"quantity"`
	OrderType  string   `json:"order_type"`
	LimitPrice *float64 `json:"limit_price,omitempty"`
	TargetPrice *float64 `json:"target_price,omitempty"`
	StopPrice  *float64 `json:"stop_price,omitempty"`
	Rationale  string   `json:"rationale"`
	Confidence float64  `json:"confidence"`
	Timeframe  string   `json:"timeframe"`
	Legs       string   `json:"legs,omitempty"`
}

type TrackRecord struct {
	TotalSuggestions  int     `json:"total_suggestions"`
	Profitable        int     `json:"profitable"`
	Unprofitable      int     `json:"unprofitable"`
	PendingOutcome    int     `json:"pending_outcome"`
	AveragePnLPercent float64 `json:"average_pnl_percent"`
	WinRate           float64 `json:"win_rate"`
}

type SuggestionManager struct {
	storage  *database.LocalStorage
	data     interfaces.DataService
	logger   *logrus.Logger
	ttlHours int
}

func NewSuggestionManager(storage *database.LocalStorage, data interfaces.DataService, logger *logrus.Logger) *SuggestionManager {
	return &SuggestionManager{
		storage:  storage,
		data:     data,
		logger:   logger,
		ttlHours: 24,
	}
}

func (sm *SuggestionManager) CreateSuggestion(req CreateSuggestionRequest, beatID int64, agentName string) (string, error) {
	id := uuid.New().String()[:8]
	now := time.Now()

	var priceAtCreation float64
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if q, err := sm.data.GetLatestQuote(ctx, req.Symbol); err == nil && q != nil {
		priceAtCreation = (q.BidPrice + q.AskPrice) / 2
		if priceAtCreation == 0 {
			priceAtCreation = q.BidPrice
		}
	}

	suggestion := &models.DBSuggestion{
		SuggestionID:    id,
		BeatID:          beatID,
		AgentName:       agentName,
		Symbol:          req.Symbol,
		Side:            req.Side,
		ActionType:      req.ActionType,
		Quantity:        req.Quantity,
		OrderType:       req.OrderType,
		LimitPrice:      req.LimitPrice,
		TargetPrice:     req.TargetPrice,
		StopPrice:       req.StopPrice,
		Rationale:       req.Rationale,
		Confidence:      req.Confidence,
		Timeframe:       req.Timeframe,
		Legs:            req.Legs,
		PriceAtCreation: priceAtCreation,
		Status:          "PENDING",
		ExpiresAt:       now.Add(time.Duration(sm.ttlHours) * time.Hour),
	}

	if err := sm.storage.SaveSuggestion(suggestion); err != nil {
		return "", fmt.Errorf("save suggestion: %w", err)
	}

	sm.logger.WithFields(logrus.Fields{
		"suggestion_id": id,
		"symbol":        req.Symbol,
		"side":          req.Side,
		"action":        req.ActionType,
		"confidence":    req.Confidence,
	}).Info("Created trade suggestion")

	b, _ := json.Marshal(suggestion)
	go appendJSONToBotLog("suggestion_created", string(b))

	return id, nil
}

func (sm *SuggestionManager) ListPending() ([]*models.DBSuggestion, error) {
	return sm.storage.GetSuggestions("PENDING")
}

func (sm *SuggestionManager) ListAll(status string) ([]*models.DBSuggestion, error) {
	return sm.storage.GetSuggestions(status)
}

func (sm *SuggestionManager) AcceptSuggestion(id string) error {
	now := time.Now()
	if err := sm.storage.UpdateSuggestionStatus(id, "ACCEPTED", &now); err != nil {
		return err
	}
	sm.logger.WithField("suggestion_id", id).Info("Suggestion accepted")
	go appendJSONToBotLog("suggestion_resolved", fmt.Sprintf(`{"id":"%s","status":"ACCEPTED"}`, id))
	return nil
}

func (sm *SuggestionManager) DismissSuggestion(id string) error {
	now := time.Now()
	if err := sm.storage.UpdateSuggestionStatus(id, "DISMISSED", &now); err != nil {
		return err
	}
	sm.logger.WithField("suggestion_id", id).Info("Suggestion dismissed")
	go appendJSONToBotLog("suggestion_resolved", fmt.Sprintf(`{"id":"%s","status":"DISMISSED"}`, id))
	return nil
}

func (sm *SuggestionManager) StartExpirationSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sm.logger.Info("Starting suggestion expiration sweeper")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sm.sweepExpired()
		}
	}
}

func (sm *SuggestionManager) sweepExpired() {
	pending, err := sm.storage.GetSuggestions("PENDING")
	if err != nil {
		return
	}
	now := time.Now()
	for _, s := range pending {
		if now.After(s.ExpiresAt) {
			_ = sm.storage.UpdateSuggestionStatus(s.SuggestionID, "EXPIRED", &now)
			sm.logger.WithField("suggestion_id", s.SuggestionID).Info("Suggestion expired")
			go appendJSONToBotLog("suggestion_resolved", fmt.Sprintf(`{"id":"%s","status":"EXPIRED"}`, s.SuggestionID))
		}
	}
}

func (sm *SuggestionManager) ReconcileOutcomes(ctx context.Context) {
	suggestions, err := sm.storage.GetSuggestionsForOutcomeCheck()
	if err != nil {
		sm.logger.WithError(err).Warn("Failed to get suggestions for outcome check")
		return
	}

	for _, s := range suggestions {
		if ctx.Err() != nil {
			return
		}
		qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		q, err := sm.data.GetLatestQuote(qCtx, s.Symbol)
		cancel()
		if err != nil || q == nil {
			continue
		}

		currentPrice := (q.BidPrice + q.AskPrice) / 2
		if currentPrice == 0 {
			currentPrice = q.BidPrice
		}
		if currentPrice == 0 || s.PriceAtCreation == 0 {
			continue
		}

		var pnl float64
		if s.Side == "BUY" || s.Side == "buy" {
			pnl = ((currentPrice - s.PriceAtCreation) / s.PriceAtCreation) * 100
		} else {
			pnl = ((s.PriceAtCreation - currentPrice) / s.PriceAtCreation) * 100
		}

		_ = sm.storage.UpdateSuggestionOutcome(s.SuggestionID, currentPrice, pnl)
	}
}

func (sm *SuggestionManager) GetTrackRecord() (*TrackRecord, error) {
	all, err := sm.storage.GetSuggestions("")
	if err != nil {
		return nil, err
	}

	record := &TrackRecord{}
	var totalPnL float64
	var evaluated int

	for _, s := range all {
		record.TotalSuggestions++
		if s.OutcomePnL == nil {
			if s.Status != "PENDING" {
				record.PendingOutcome++
			}
			continue
		}
		evaluated++
		if *s.OutcomePnL > 0 {
			record.Profitable++
		} else {
			record.Unprofitable++
		}
		totalPnL += *s.OutcomePnL
	}

	if evaluated > 0 {
		record.AveragePnLPercent = totalPnL / float64(evaluated)
		record.WinRate = float64(record.Profitable) / float64(evaluated) * 100
	}

	return record, nil
}
