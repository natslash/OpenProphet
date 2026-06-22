package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"prophet-trader/models"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ManagedPosition represents a position with automated risk management
type ManagedPosition struct {
	ID                string                 `json:"id"`
	Symbol            string                 `json:"symbol"`
	Side              string                 `json:"side"` // "buy" or "sell"
	Strategy          string                 `json:"strategy"` // "SWING_TRADE", "LONG_TERM", "DAY_TRADE"

	// Entry details
	Quantity          float64                `json:"quantity"`
	EntryPrice        float64                `json:"entry_price"`
	EntryOrderID      string                 `json:"entry_order_id"`
	EntryOrderType    string                 `json:"entry_order_type"` // "market", "limit"
	AllocationDollars float64                `json:"allocation_dollars"`

	// Risk management
	StopLossPrice     float64                `json:"stop_loss_price"`
	StopLossPercent   float64                `json:"stop_loss_percent"`
	StopLossOrderID   string                 `json:"stop_loss_order_id,omitempty"`
	TrailingStop      bool                   `json:"trailing_stop"`
	TrailingPercent   float64                `json:"trailing_percent,omitempty"`

	// Profit targets
	TakeProfitPrice   float64                `json:"take_profit_price"`
	TakeProfitPercent float64                `json:"take_profit_percent"`
	TakeProfitOrderID string                 `json:"take_profit_order_id,omitempty"`

	// Partial exit strategy
	PartialExit       *PartialExitConfig     `json:"partial_exit,omitempty"`
	PartialExitOrders []string               `json:"partial_exit_orders,omitempty"`

	// Status tracking
	Status            string                 `json:"status"` // "PENDING", "ACTIVE", "PARTIAL", "CLOSED", "STOPPED_OUT", "FAILED"
	CurrentPrice      float64                `json:"current_price"`
	UnrealizedPL      float64                `json:"unrealized_pl"`
	UnrealizedPLPC    float64                `json:"unrealized_pl_percent"`
	RemainingQty      float64                `json:"remaining_qty"`

	// Metadata
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	ClosedAt          *time.Time             `json:"closed_at,omitempty"`
	Notes             string                 `json:"notes,omitempty"`
	Tags              []string               `json:"tags,omitempty"`
}

// PartialExitConfig defines partial profit taking strategy
type PartialExitConfig struct {
	Enabled       bool    `json:"enabled"`
	Percent       float64 `json:"percent"`        // % of position to exit
	TargetPercent float64 `json:"target_percent"` // % gain to trigger partial exit
	TargetPrice   float64 `json:"target_price"`   // Calculated target price
}

// PlaceManagedPositionRequest represents request to open a managed position
type PlaceManagedPositionRequest struct {
	Symbol            string              `json:"symbol" binding:"required"`
	Side              string              `json:"side" binding:"required"` // "buy" or "sell"
	Strategy          string              `json:"strategy"` // "SWING_TRADE", "LONG_TERM", "DAY_TRADE"
	AllocationDollars float64             `json:"allocation_dollars"`
	// ExplicitQuantity, when set, bypasses dollar-allocation sizing and fixes
	// the order quantity exactly. The autonomous beat uses it to enforce the
	// 1-lot hard cap at the execution boundary (validated to be exactly 1).
	ExplicitQuantity  *int                `json:"explicit_quantity,omitempty"`

	// Entry configuration
	EntryStrategy     string              `json:"entry_strategy"` // "market", "limit"
	EntryPrice        *float64            `json:"entry_price,omitempty"` // Required for limit orders

	// Risk management (one of these required)
	StopLossPrice     *float64            `json:"stop_loss_price,omitempty"`
	StopLossPercent   *float64            `json:"stop_loss_percent,omitempty"`
	TrailingStop      bool                `json:"trailing_stop"`
	TrailingPercent   float64             `json:"trailing_percent,omitempty"`

	// Profit targets (one of these required)
	TakeProfitPrice   *float64            `json:"take_profit_price,omitempty"`
	TakeProfitPercent *float64            `json:"take_profit_percent,omitempty"`

	// Partial exit (optional)
	PartialExit       *PartialExitConfig  `json:"partial_exit,omitempty"`

	// Metadata
	Notes             string              `json:"notes,omitempty"`
	Tags              []string            `json:"tags,omitempty"`
}

// PositionManager handles automated position management
type PositionManager struct {
	tradingService interfaces.TradingService
	dataService    interfaces.DataService
	storageService *database.LocalStorage

	positions      map[string]*ManagedPosition // position_id -> position
	mu             sync.RWMutex
	logger         *logrus.Logger

	ctx            context.Context
	cancel         context.CancelFunc
}

// NewPositionManager creates a new position manager
func NewPositionManager(
	tradingService interfaces.TradingService,
	dataService interfaces.DataService,
	storageService *database.LocalStorage,
) *PositionManager {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	ctx, cancel := context.WithCancel(context.Background())

	pm := &PositionManager{
		tradingService: tradingService,
		dataService:    dataService,
		storageService: storageService,
		positions:      make(map[string]*ManagedPosition),
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Load existing positions from database
	if err := pm.loadPositionsFromDB(); err != nil {
		logger.WithError(err).Error("Failed to load positions from database")
	}

	return pm
}

// PlaceManagedPosition opens a new managed position with automated risk management
func (pm *PositionManager) PlaceManagedPosition(ctx context.Context, req *PlaceManagedPositionRequest) (*ManagedPosition, error) {
	pm.logger.WithFields(logrus.Fields{
		"symbol":     req.Symbol,
		"side":       req.Side,
		"allocation": req.AllocationDollars,
	}).Info("Placing managed position")

	// Validate request
	if err := pm.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Get current price for calculations
	currentPrice, err := pm.getCurrentPrice(ctx, req.Symbol)
	if err != nil {
		// Graceful degradation: If we have an explicit entry limit price, use that as the baseline
		// instead of completely failing the placement (especially useful for off-hours/weekend trading).
		if req.EntryPrice != nil {
			pm.logger.WithError(err).Warn("Failed to fetch live quote, falling back to explicit entry price")
			currentPrice = *req.EntryPrice
		} else {
			return nil, fmt.Errorf("failed to get current price and no entry price provided: %w", err)
		}
	}

	// Calculate position parameters
	entryPrice := currentPrice
	if req.EntryPrice != nil {
		entryPrice = *req.EntryPrice
	}

	quantity := pm.resolveQuantity(req, entryPrice)

	// Calculate stop loss
	stopLossPrice := pm.calculateStopLoss(entryPrice, req.StopLossPrice, req.StopLossPercent, req.Side)
	stopLossPercent := math.Abs((stopLossPrice - entryPrice) / entryPrice * 100)

	// Calculate take profit
	takeProfitPrice := pm.calculateTakeProfit(entryPrice, req.TakeProfitPrice, req.TakeProfitPercent, req.Side)
	takeProfitPercent := math.Abs((takeProfitPrice - entryPrice) / entryPrice * 100)

	// Calculate partial exit if configured
	if req.PartialExit != nil && req.PartialExit.Enabled {
		req.PartialExit.TargetPrice = pm.calculatePartialExitPrice(entryPrice, req.PartialExit.TargetPercent, req.Side)
	}

	// Create managed position
	position := &ManagedPosition{
		ID:                pm.generatePositionID(),
		Symbol:            req.Symbol,
		Side:              req.Side,
		Strategy:          req.Strategy,
		Quantity:          quantity,
		EntryPrice:        entryPrice,
		EntryOrderType:    req.EntryStrategy,
		AllocationDollars: req.AllocationDollars,
		StopLossPrice:     stopLossPrice,
		StopLossPercent:   stopLossPercent,
		TrailingStop:      req.TrailingStop,
		TrailingPercent:   req.TrailingPercent,
		TakeProfitPrice:   takeProfitPrice,
		TakeProfitPercent: takeProfitPercent,
		PartialExit:       req.PartialExit,
		Status:            "PENDING",
		CurrentPrice:      currentPrice,
		RemainingQty:      quantity,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Notes:             req.Notes,
		Tags:              req.Tags,
	}

	// Place entry order
	if err := pm.placeEntryOrder(ctx, position); err != nil {
		return nil, fmt.Errorf("failed to place entry order: %w", err)
	}

	// Store position
	pm.mu.Lock()
	pm.positions[position.ID] = position
	pm.mu.Unlock()

	// Save to database
	if err := pm.savePositionToDB(position); err != nil {
		pm.logger.WithError(err).Error("Failed to save position to database")
	}

	pm.logger.WithFields(logrus.Fields{
		"position_id":       position.ID,
		"entry_order_id":    position.EntryOrderID,
		"quantity":          quantity,
		"entry_price":       entryPrice,
		"stop_loss":         stopLossPrice,
		"take_profit":       takeProfitPrice,
		"risk_reward_ratio": takeProfitPercent / stopLossPercent,
	}).Info("Managed position created")

	return position, nil
}

// placeEntryOrder places the initial entry order
func (pm *PositionManager) placeEntryOrder(ctx context.Context, position *ManagedPosition) error {
	orderType := "market"
	if position.EntryOrderType == "limit" {
		orderType = "limit"
	}

	order := &interfaces.Order{
		Symbol:      position.Symbol,
		Qty:         position.Quantity,
		Side:        position.Side,
		Type:        orderType,
		TimeInForce: "gtc",
		Status:      "pending",
		SubmittedAt: time.Now(),
	}

	if orderType == "limit" {
		order.LimitPrice = &position.EntryPrice
	}

	if position.TakeProfitPrice > 0 {
		order.TakeProfitPrice = &position.TakeProfitPrice
	}
	if position.StopLossPrice > 0 {
		order.StopLossPrice = &position.StopLossPrice
	}

	result, err := pm.tradingService.PlaceOrder(ctx, order)
	if err != nil {
		return err
	}

	position.EntryOrderID = result.OrderID
	position.TakeProfitOrderID = result.TakeProfitOrderID
	position.StopLossOrderID = result.StopLossOrderID
	position.Status = "PENDING"

	return nil
}

// MonitorPositions monitors all active positions and manages risk
func (pm *PositionManager) MonitorPositions(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	pm.logger.Info("Position monitoring started")

	for {
		select {
		case <-ctx.Done():
			pm.logger.Info("Position monitoring stopped")
			return
		case <-ticker.C:
			pm.checkPositions(ctx)
		}
	}
}

// checkPositions checks all positions and manages their risk orders
func (pm *PositionManager) checkPositions(ctx context.Context) {
	// 1. Poll live quotes for all active positions first
	if err := pm.pollActiveQuotes(ctx); err != nil {
		if err.Error() == "HTTP 429: Too Many Requests" || err.Error() == "HTTP 429" {
			pm.logger.Warn("Rate limited by IBKR, pausing checkPositions for backoff")
			time.Sleep(5 * time.Second) // Exponential backoff in caller loop
			return
		}
	}

	pm.mu.RLock()
	positions := make([]*ManagedPosition, 0, len(pm.positions))
	for _, pos := range pm.positions {
		positions = append(positions, pos)
	}
	pm.mu.RUnlock()

	for _, position := range positions {
		if position.Status == "CLOSED" || position.Status == "STOPPED_OUT" {
			continue
		}

		// Check if entry order filled
		if position.Status == "PENDING" {
			pm.checkEntryOrder(ctx, position)
			continue
		}

		// Check if we need to place/update risk orders
		if position.Status == "ACTIVE" {
			pm.manageRiskOrders(ctx, position)
		}

		// Check trailing stop
		if position.TrailingStop {
			pm.updateTrailingStop(ctx, position)
		}
	}
}

// checkEntryOrder checks if entry order has filled
func (pm *PositionManager) checkEntryOrder(ctx context.Context, position *ManagedPosition) {
	order, err := pm.tradingService.GetOrder(ctx, position.EntryOrderID)
	if err != nil {
		// Fallback: order may have been filled while backend was offline
		brokerPositions, pErr := pm.tradingService.GetPositions(ctx)
		if pErr == nil {
			for _, bp := range brokerPositions {
				if bp.Symbol == position.Symbol && ((position.Side == "buy" && bp.Qty > 0) || (position.Side == "sell" && bp.Qty < 0)) {
					pm.logger.WithFields(logrus.Fields{
						"position_id": position.ID,
						"symbol":      position.Symbol,
						"avg_cost":    bp.AvgEntryPrice,
					}).Info("Entry order missing but native position exists; marking ACTIVE")

					position.Status = "ACTIVE"
					position.EntryPrice = bp.AvgEntryPrice
					position.UpdatedAt = time.Now()

					if position.PartialExit != nil && position.PartialExit.Enabled {
						if err := pm.placePartialExitOrder(ctx, position); err != nil {
							pm.logger.WithError(err).Error("Failed to place partial exit order")
						}
					}
					pm.savePositionToDB(position)
					return
				}
			}
		}

		pm.logger.WithFields(logrus.Fields{
			"position_id": position.ID,
			"order_id":    position.EntryOrderID,
			"symbol":      position.Symbol,
		}).Warn("Entry order not found and no matching broker position; marking CLOSED as stale")
		position.Status = "CLOSED"
		position.Notes = "Auto-closed: entry order lost after restart"
		position.UpdatedAt = time.Now()
		pm.savePositionToDB(position)
		return
	}

	if order.Status == "filled" || strings.ToLower(order.Status) == "filled" {
		position.Status = "ACTIVE"
		position.EntryPrice = *order.FilledAvgPrice
		position.UpdatedAt = time.Now()

		pm.logger.WithFields(logrus.Fields{
			"position_id": position.ID,
			"symbol":      position.Symbol,
			"fill_price":  position.EntryPrice,
		}).Info("Entry order filled - native bracket risk active")

		// Place partial exit order if configured (since it's not natively part of the bracket)
		if position.PartialExit != nil && position.PartialExit.Enabled {
			if err := pm.placePartialExitOrder(ctx, position); err != nil {
				pm.logger.WithError(err).Error("Failed to place partial exit order")
			}
		}

		// Save to database
		pm.savePositionToDB(position)
	}
}

// placeRiskOrders, placeStopLossOrder, and placeTakeProfitOrder deleted - native brackets now used.

// placePartialExitOrder places partial exit order
func (pm *PositionManager) placePartialExitOrder(ctx context.Context, position *ManagedPosition) error {
	exitSide := "sell"
	if position.Side == "sell" {
		exitSide = "buy"
	}

	partialQty := position.Quantity * (position.PartialExit.Percent / 100.0)

	order := &interfaces.Order{
		Symbol:      position.Symbol,
		Qty:         partialQty,
		Side:        exitSide,
		Type:        "limit",
		TimeInForce: "gtc",
		LimitPrice:  &position.PartialExit.TargetPrice,
		Status:      "pending",
		SubmittedAt: time.Now(),
	}

	result, err := pm.tradingService.PlaceOrder(ctx, order)
	if err != nil {
		return err
	}

	position.PartialExitOrders = append(position.PartialExitOrders, result.OrderID)
	pm.logger.WithFields(logrus.Fields{
		"position_id": position.ID,
		"order_id":    result.OrderID,
		"quantity":    partialQty,
		"limit_price": position.PartialExit.TargetPrice,
	}).Info("Partial exit order placed")

	return nil
}

// manageRiskOrders checks and updates risk management orders
func (pm *PositionManager) manageRiskOrders(ctx context.Context, position *ManagedPosition) {
	// Deterministic Programmatic Fallback Rules
	if position.CurrentPrice > 0 {
		hitStopLoss := (position.Side == "buy" && position.CurrentPrice <= position.StopLossPrice) ||
			(position.Side == "sell" && position.CurrentPrice >= position.StopLossPrice)

		hitTakeProfit := (position.TakeProfitPrice > 0) &&
			((position.Side == "buy" && position.CurrentPrice >= position.TakeProfitPrice) ||
				(position.Side == "sell" && position.CurrentPrice <= position.TakeProfitPrice))

		if hitTakeProfit {
			pm.logger.WithFields(logrus.Fields{
				"position_id":   position.ID,
				"current_price": position.CurrentPrice,
				"target_price":  position.TakeProfitPrice,
			}).Info("Programmatic Take Profit Triggered - Executing Market Sell")
			
			// We trigger CloseManagedPosition which handles cancelling brackets and placing the market exit
			pm.CloseManagedPosition(ctx, position.ID)
			return
		}

		if hitStopLoss {
			pm.logger.WithFields(logrus.Fields{
				"position_id":   position.ID,
				"current_price": position.CurrentPrice,
				"stop_price":    position.StopLossPrice,
			}).Info("Programmatic Stop Loss Triggered - Executing Market Sell")
			
			pm.CloseManagedPosition(ctx, position.ID)
			return
		}
	}

	// Fallback for native brackets: if the orders vanish or fill, the broker position goes to 0.
	brokerPositions, err := pm.tradingService.GetPositions(ctx)
	if err == nil {
		var foundQty float64
		for _, bp := range brokerPositions {
			if bp.Symbol == position.Symbol {
				foundQty = bp.Qty
				break
			}
		}

		if foundQty == 0 {
			position.Status = "CLOSED"
			position.RemainingQty = 0
			now := time.Now()
			position.ClosedAt = &now
			pm.logger.WithField("position_id", position.ID).Info("Position natively closed by bracket")
			pm.savePositionToDB(position)
			return
		}
	}

	// Also explicitly check stop loss order status (if ID is known and it's still available in GetOrder)
	if position.StopLossOrderID != "" {
		order, err := pm.tradingService.GetOrder(ctx, position.StopLossOrderID)
		if err == nil && order.Status == "filled" {
			position.Status = "STOPPED_OUT"
			now := time.Now()
			position.ClosedAt = &now
			position.RemainingQty = 0
			pm.logger.WithField("position_id", position.ID).Info("Position stopped out natively")
			pm.savePositionToDB(position)
			return
		}
	}

	// Check take profit order status
	if position.TakeProfitOrderID != "" {
		order, err := pm.tradingService.GetOrder(ctx, position.TakeProfitOrderID)
		if err == nil && order.Status == "filled" {
			position.Status = "CLOSED"
			now := time.Now()
			position.ClosedAt = &now
			position.RemainingQty = 0
			pm.logger.WithField("position_id", position.ID).Info("Position closed natively at profit target")
			pm.savePositionToDB(position)
			return
		}
	}

	// Check partial exit orders
	for _, orderID := range position.PartialExitOrders {
		order, err := pm.tradingService.GetOrder(ctx, orderID)
		if err == nil && order.Status == "filled" {
			position.Status = "PARTIAL"
			position.RemainingQty -= order.FilledQty
			pm.logger.WithFields(logrus.Fields{
				"position_id":   position.ID,
				"filled_qty":    order.FilledQty,
				"remaining_qty": position.RemainingQty,
			}).Info("Partial exit filled")
			pm.savePositionToDB(position)
		}
	}
}

// updateTrailingStop updates trailing stop loss based on current price
func (pm *PositionManager) updateTrailingStop(ctx context.Context, position *ManagedPosition) {
	// TODO(Phase 4.4): Native trailing stops for brackets require sending an Order Modification
	// for the specific SL child leg instead of placing a brand new order.
	pm.logger.WithField("position_id", position.ID).Warn("Trailing stops are currently disabled pending native bracket modification support.")
}

// updatePositionPrice updates current price and unrealized P&L
func (pm *PositionManager) updatePositionPrice(ctx context.Context, position *ManagedPosition) error {
	currentPrice, err := pm.getCurrentPrice(ctx, position.Symbol)
	if err != nil {
		return err
	}

	position.CurrentPrice = currentPrice

	if position.Side == "buy" {
		position.UnrealizedPL = (currentPrice - position.EntryPrice) * position.RemainingQty
		position.UnrealizedPLPC = ((currentPrice - position.EntryPrice) / position.EntryPrice) * 100
	} else {
		position.UnrealizedPL = (position.EntryPrice - currentPrice) * position.RemainingQty
		position.UnrealizedPLPC = ((position.EntryPrice - currentPrice) / position.EntryPrice) * 100
	}

	position.UpdatedAt = time.Now()

	return nil
}

// pollActiveQuotes iterates over active positions and updates their prices
func (pm *PositionManager) pollActiveQuotes(ctx context.Context) error {
	pm.mu.RLock()
	var activePositions []*ManagedPosition
	for _, pos := range pm.positions {
		if pos.Status == "ACTIVE" || pos.Status == "PARTIAL" {
			activePositions = append(activePositions, pos)
		}
	}
	pm.mu.RUnlock()

	for _, pos := range activePositions {
		if err := pm.updatePositionPrice(ctx, pos); err != nil {
			pm.logger.WithError(err).WithField("symbol", pos.Symbol).Warn("Failed to poll quote")
			// Return immediately on rate limit to trigger backoff
			if strings.Contains(err.Error(), "HTTP 429") {
				return err
			}
		}
	}
	return nil
}

// GetManagedPosition retrieves a managed position by ID
func (pm *PositionManager) GetManagedPosition(positionID string) (*ManagedPosition, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	position, exists := pm.positions[positionID]
	if !exists {
		return nil, fmt.Errorf("position not found: %s", positionID)
	}

	return position, nil
}

// ListManagedPositions returns all managed positions
// Filters out PENDING positions older than 24 hours (stale orders)
func (pm *PositionManager) ListManagedPositions(status string) []*ManagedPosition {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	positions := make([]*ManagedPosition, 0)
	now := time.Now()

	for _, pos := range pm.positions {
		// Filter out stale PENDING orders (>24 hours old)
		if pos.Status == "PENDING" {
			age := now.Sub(pos.CreatedAt)
			if age > 24*time.Hour {
				pm.logger.WithFields(logrus.Fields{
					"position_id": pos.ID,
					"symbol":      pos.Symbol,
					"age_hours":   age.Hours(),
				}).Debug("Skipping stale PENDING position")
				continue
			}
		}

		if status == "" || pos.Status == status {
			positions = append(positions, pos)
		}
	}

	return positions
}

// CloseManagedPosition manually closes a managed position
func (pm *PositionManager) CloseManagedPosition(ctx context.Context, positionID string) error {
	pm.mu.RLock()
	position, exists := pm.positions[positionID]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("position not found: %s", positionID)
	}

	// Cancel all open orders (ignore errors - orders may already be cancelled or market closed)

	// Cancel entry order if still pending
	if position.EntryOrderID != "" {
		if err := pm.tradingService.CancelOrder(ctx, position.EntryOrderID); err != nil {
			pm.logger.WithError(err).Warn("Failed to cancel entry order (may already be filled/cancelled)")
		} else {
			pm.logger.WithField("order_id", position.EntryOrderID).Info("Cancelled entry order")
		}
	}

	if position.StopLossOrderID != "" {
		if err := pm.tradingService.CancelOrder(ctx, position.StopLossOrderID); err != nil {
			pm.logger.WithError(err).Warn("Failed to cancel stop loss order (may already be cancelled)")
		} else {
			pm.logger.WithField("order_id", position.StopLossOrderID).Info("Cancelled stop loss order")
		}
	}
	if position.TakeProfitOrderID != "" {
		if err := pm.tradingService.CancelOrder(ctx, position.TakeProfitOrderID); err != nil {
			pm.logger.WithError(err).Warn("Failed to cancel take profit order (may already be cancelled)")
		} else {
			pm.logger.WithField("order_id", position.TakeProfitOrderID).Info("Cancelled take profit order")
		}
	}
	for _, orderID := range position.PartialExitOrders {
		if err := pm.tradingService.CancelOrder(ctx, orderID); err != nil {
			pm.logger.WithError(err).Warn("Failed to cancel partial exit order (may already be cancelled)")
		} else {
			pm.logger.WithField("order_id", orderID).Info("Cancelled partial exit order")
		}
	}

	// Place market order to close remaining position (ONLY if position is ACTIVE/PARTIAL - i.e., entry was filled)
	if position.Status == "ACTIVE" || position.Status == "PARTIAL" {
		if position.RemainingQty > 0 {
			exitSide := "sell"
			if position.Side == "sell" {
				exitSide = "buy"
			}

			order := &interfaces.Order{
				Symbol:      position.Symbol,
				Qty:         position.RemainingQty,
				Side:        exitSide,
				Type:        "market",
				TimeInForce: "day",
				Status:      "pending",
				SubmittedAt: time.Now(),
			}

			_, err := pm.tradingService.PlaceOrder(ctx, order)
			if err != nil {
				// Log error but still close the position in our system
				pm.logger.WithError(err).Error("Failed to place exit order (market may be closed)")
				pm.logger.Info("Closing position in database despite order error")
			} else {
				pm.logger.WithField("quantity", position.RemainingQty).Info("Placed market exit order")
			}
		}
	} else if position.Status == "PENDING" {
		// For pending positions, just log that we cancelled the entry order
		pm.logger.WithField("position_id", position.ID).Info("Closed pending position (entry order was never filled)")
	}

	position.Status = "CLOSED"
	now := time.Now()
	position.ClosedAt = &now

	// Save to database
	pm.savePositionToDB(position)

	pm.logger.WithField("position_id", positionID).Info("Position manually closed")

	return nil
}

// Helper functions

func (pm *PositionManager) validateRequest(req *PlaceManagedPositionRequest) error {
	if req.Side != "buy" && req.Side != "sell" {
		return fmt.Errorf("side must be 'buy' or 'sell'")
	}

	// Sizing: an explicit quantity (autonomous beat) is hard-capped at exactly
	// 1 lot; otherwise a positive dollar allocation is required.
	if req.ExplicitQuantity != nil {
		if *req.ExplicitQuantity != 1 {
			return fmt.Errorf("explicit_quantity must be exactly 1 (1-lot hard cap); got %d", *req.ExplicitQuantity)
		}
	} else if req.AllocationDollars <= 0 {
		return fmt.Errorf("allocation_dollars must be > 0 (or set explicit_quantity)")
	}

	if req.EntryStrategy == "limit" && req.EntryPrice == nil {
		return fmt.Errorf("entry_price required for limit orders")
	}

	if req.StopLossPrice == nil && req.StopLossPercent == nil {
		return fmt.Errorf("either stop_loss_price or stop_loss_percent required")
	}

	if req.TakeProfitPrice == nil && req.TakeProfitPercent == nil {
		return fmt.Errorf("either take_profit_price or take_profit_percent required")
	}

	return nil
}

func (pm *PositionManager) getCurrentPrice(ctx context.Context, symbol string) (float64, error) {
	quote, err := pm.dataService.GetLatestQuote(ctx, symbol)
	if err == nil {
		if quote.AskPrice > 0 {
			return quote.AskPrice, nil
		}
		if quote.BidPrice > 0 {
			return quote.BidPrice, nil
		}
	}

	// Fallback: live quotes may not be entitled (e.g. IBKR paper returns error
	// 10089). Use the latest historical bar's close instead.
	if bar, berr := pm.dataService.GetLatestBar(ctx, symbol); berr == nil && bar != nil && bar.Close > 0 {
		return bar.Close, nil
	}

	if err != nil {
		return 0, fmt.Errorf("no usable price for %s (live quote failed and no bar): %w", symbol, err)
	}
	return 0, fmt.Errorf("no usable price for %s", symbol)
}

func (pm *PositionManager) calculateQuantity(allocation, price float64) float64 {
	return math.Floor(allocation / price)
}

// resolveQuantity returns the exact ExplicitQuantity when set (the autonomous
// beat's 1-lot cap), otherwise sizes by dollar allocation.
func (pm *PositionManager) resolveQuantity(req *PlaceManagedPositionRequest, price float64) float64 {
	if req.ExplicitQuantity != nil {
		return float64(*req.ExplicitQuantity)
	}
	return pm.calculateQuantity(req.AllocationDollars, price)
}

func (pm *PositionManager) calculateStopLoss(entryPrice float64, stopPrice *float64, stopPercent *float64, side string) float64 {
	if stopPrice != nil {
		return pm.roundPrice(*stopPrice)
	}

	if side == "buy" {
		return pm.roundPrice(entryPrice * (1 - *stopPercent/100.0))
	}

	return pm.roundPrice(entryPrice * (1 + *stopPercent/100.0))
}

func (pm *PositionManager) roundPrice(val float64) float64 {
	return math.Round(val*10) / 10
}

func (pm *PositionManager) calculateTakeProfit(entryPrice float64, profitPrice *float64, profitPercent *float64, side string) float64 {
	if profitPrice != nil {
		return pm.roundPrice(*profitPrice)
	}

	if side == "buy" {
		return pm.roundPrice(entryPrice * (1 + *profitPercent/100.0))
	}

	return pm.roundPrice(entryPrice * (1 - *profitPercent/100.0))
}

func (pm *PositionManager) calculatePartialExitPrice(entryPrice, targetPercent float64, side string) float64 {
	if side == "buy" {
		return entryPrice * (1 + targetPercent/100.0)
	}

	return entryPrice * (1 - targetPercent/100.0)
}

func (pm *PositionManager) generatePositionID() string {
	return fmt.Sprintf("pos_%d", time.Now().UnixNano())
}

// Stop stops the position manager
func (pm *PositionManager) Stop() {
	pm.cancel()
}

// loadPositionsFromDB loads all active positions from database on startup
func (pm *PositionManager) loadPositionsFromDB() error {
	// Load all non-closed positions
	dbPositions, err := pm.storageService.GetAllManagedPositions("")
	if err != nil {
		return err
	}

	loaded := 0
	for _, dbPos := range dbPositions {
		// Skip closed positions
		if dbPos.Status == "CLOSED" || dbPos.Status == "STOPPED_OUT" {
			continue
		}

		// Convert DB position to managed position
		position := pm.dbToManagedPosition(dbPos)

		// Store in memory
		pm.positions[position.ID] = position
		loaded++
	}

	pm.logger.WithField("count", loaded).Info("Loaded managed positions from database")
	return nil
}

// savePositionToDB saves a managed position to database
func (pm *PositionManager) savePositionToDB(position *ManagedPosition) error {
	dbPosition := pm.managedPositionToDB(position)
	return pm.storageService.SaveManagedPosition(dbPosition)
}

// managedPositionToDB converts ManagedPosition to DBManagedPosition
func (pm *PositionManager) managedPositionToDB(pos *ManagedPosition) *models.DBManagedPosition {
	// Convert partial exit orders to JSON
	partialExitOrdersJSON, _ := json.Marshal(pos.PartialExitOrders)

	// Convert tags to JSON
	tagsJSON, _ := json.Marshal(pos.Tags)

	dbPos := &models.DBManagedPosition{
		PositionID:        pos.ID,
		Symbol:            pos.Symbol,
		Side:              pos.Side,
		Strategy:          pos.Strategy,
		Quantity:          pos.Quantity,
		EntryPrice:        pos.EntryPrice,
		EntryOrderID:      pos.EntryOrderID,
		EntryOrderType:    pos.EntryOrderType,
		AllocationDollars: pos.AllocationDollars,
		StopLossPrice:     pos.StopLossPrice,
		StopLossPercent:   pos.StopLossPercent,
		StopLossOrderID:   pos.StopLossOrderID,
		TrailingStop:      pos.TrailingStop,
		TrailingPercent:   pos.TrailingPercent,
		TakeProfitPrice:   pos.TakeProfitPrice,
		TakeProfitPercent: pos.TakeProfitPercent,
		TakeProfitOrderID: pos.TakeProfitOrderID,
		Status:            pos.Status,
		CurrentPrice:      pos.CurrentPrice,
		UnrealizedPL:      pos.UnrealizedPL,
		UnrealizedPLPC:    pos.UnrealizedPLPC,
		RemainingQty:      pos.RemainingQty,
		Notes:             pos.Notes,
		Tags:              string(tagsJSON),
		PartialExitOrders: string(partialExitOrdersJSON),
		ClosedAt:          pos.ClosedAt,
	}

	if pos.PartialExit != nil {
		dbPos.PartialExitEnabled = pos.PartialExit.Enabled
		dbPos.PartialExitPercent = pos.PartialExit.Percent
		dbPos.PartialExitTargetPercent = pos.PartialExit.TargetPercent
		dbPos.PartialExitTargetPrice = pos.PartialExit.TargetPrice
	}

	return dbPos
}

// dbToManagedPosition converts DBManagedPosition to ManagedPosition
func (pm *PositionManager) dbToManagedPosition(dbPos *models.DBManagedPosition) *ManagedPosition {
	// Parse partial exit orders from JSON
	var partialExitOrders []string
	if dbPos.PartialExitOrders != "" {
		json.Unmarshal([]byte(dbPos.PartialExitOrders), &partialExitOrders)
	}

	// Parse tags from JSON
	var tags []string
	if dbPos.Tags != "" {
		json.Unmarshal([]byte(dbPos.Tags), &tags)
	}

	pos := &ManagedPosition{
		ID:                dbPos.PositionID,
		Symbol:            dbPos.Symbol,
		Side:              dbPos.Side,
		Strategy:          dbPos.Strategy,
		Quantity:          dbPos.Quantity,
		EntryPrice:        dbPos.EntryPrice,
		EntryOrderID:      dbPos.EntryOrderID,
		EntryOrderType:    dbPos.EntryOrderType,
		AllocationDollars: dbPos.AllocationDollars,
		StopLossPrice:     dbPos.StopLossPrice,
		StopLossPercent:   dbPos.StopLossPercent,
		StopLossOrderID:   dbPos.StopLossOrderID,
		TrailingStop:      dbPos.TrailingStop,
		TrailingPercent:   dbPos.TrailingPercent,
		TakeProfitPrice:   dbPos.TakeProfitPrice,
		TakeProfitPercent: dbPos.TakeProfitPercent,
		TakeProfitOrderID: dbPos.TakeProfitOrderID,
		Status:            dbPos.Status,
		CurrentPrice:      dbPos.CurrentPrice,
		UnrealizedPL:      dbPos.UnrealizedPL,
		UnrealizedPLPC:    dbPos.UnrealizedPLPC,
		RemainingQty:      dbPos.RemainingQty,
		Notes:             dbPos.Notes,
		Tags:              tags,
		PartialExitOrders: partialExitOrders,
		CreatedAt:         dbPos.CreatedAt,
		UpdatedAt:         dbPos.UpdatedAt,
		ClosedAt:          dbPos.ClosedAt,
	}

	if dbPos.PartialExitEnabled {
		pos.PartialExit = &PartialExitConfig{
			Enabled:       dbPos.PartialExitEnabled,
			Percent:       dbPos.PartialExitPercent,
			TargetPercent: dbPos.PartialExitTargetPercent,
			TargetPrice:   dbPos.PartialExitTargetPrice,
		}
	}

	return pos
}
