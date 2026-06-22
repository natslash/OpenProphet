package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type IntentStatus string

const (
	IntentStatusPending   IntentStatus = "PENDING"
	IntentStatusExecuting IntentStatus = "EXECUTING"
	IntentStatusCompleted IntentStatus = "COMPLETED"
	IntentStatusRejected  IntentStatus = "REJECTED"
	IntentStatusExpired   IntentStatus = "EXPIRED"
)

type IntentType string

const (
	IntentTypeManagedPosition IntentType = "managed_position"
	IntentTypeOptionsOrder    IntentType = "options_order"
)

type Intent struct {
	ID           string
	Type         IntentType
	Payload      []byte // Raw JSON of PlaceManagedPositionRequest or options order req
	CreatedAt    time.Time
	CurrentPrice float64
	Status       IntentStatus
	Symbol       string
	Side         string
	Quantity     float64
}

type IntentManager struct {
	intents    map[string]*Intent
	mu         sync.RWMutex
	logger     *logrus.Logger
	ttlSeconds int

	onIntentExpiredOrRejected func(intent *Intent, reason string)
}

func NewIntentManager(ttlSeconds int, logger *logrus.Logger) *IntentManager {
	im := &IntentManager{
		intents:    make(map[string]*Intent),
		logger:     logger,
		ttlSeconds: ttlSeconds,
	}
	return im
}

func (im *IntentManager) SetFeedbackCallback(cb func(intent *Intent, reason string)) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.onIntentExpiredOrRejected = cb
}

func (im *IntentManager) StartSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	im.logger.WithField("ttl_seconds", im.ttlSeconds).Info("Starting IntentManager TTL sweeper")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			im.sweep()
		}
	}
}

func (im *IntentManager) sweep() {
	im.mu.Lock()
	defer im.mu.Unlock()

	now := time.Now()
	for id, intent := range im.intents {
		if intent.Status == IntentStatusPending {
			if now.Sub(intent.CreatedAt).Seconds() > float64(im.ttlSeconds) {
				im.logger.WithField("intent_id", id).Warn("Intent expired due to TTL")
				intent.Status = IntentStatusExpired
				
				if im.onIntentExpiredOrRejected != nil {
					// Don't block the sweeper
					go im.onIntentExpiredOrRejected(intent, "expired due to TTL")
				}
				
				delete(im.intents, id)
			}
		}
	}
}

func (im *IntentManager) CreateIntent(intentType IntentType, payload []byte, symbol string, side string, qty float64, currentPrice float64) (string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	id := uuid.New().String()
	intent := &Intent{
		ID:           id,
		Type:         intentType,
		Payload:      payload,
		CreatedAt:    time.Now(),
		CurrentPrice: currentPrice,
		Status:       IntentStatusPending,
		Symbol:       symbol,
		Side:         side,
		Quantity:     qty,
	}

	im.intents[id] = intent
	im.logger.WithFields(logrus.Fields{
		"intent_id": id,
		"type":      intentType,
		"symbol":    symbol,
		"side":      side,
		"qty":       qty,
		"price":     currentPrice,
	}).Info("Created new trading intent, pending human authorization")

	return id, nil
}

func (im *IntentManager) GetIntent(id string) (*Intent, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	intent, exists := im.intents[id]
	if !exists {
		return nil, fmt.Errorf("intent not found: %s", id)
	}
	return intent, nil
}

func (im *IntentManager) ListIntents() []*Intent {
	im.mu.RLock()
	defer im.mu.RUnlock()

	list := make([]*Intent, 0, len(im.intents))
	for _, intent := range im.intents {
		list = append(list, intent)
	}
	return list
}

// ClaimForExecution atomically transitions a Pending intent to Executing.
func (im *IntentManager) ClaimForExecution(id string) (*Intent, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	intent, exists := im.intents[id]
	if !exists {
		return nil, fmt.Errorf("intent not found: %s", id)
	}

	if intent.Status != IntentStatusPending {
		return nil, fmt.Errorf("intent is not pending (status: %s)", intent.Status)
	}

	intent.Status = IntentStatusExecuting
	return intent, nil
}

func (im *IntentManager) MarkCompleted(id string) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if intent, exists := im.intents[id]; exists {
		intent.Status = IntentStatusCompleted
		delete(im.intents, id)
	}
}

func (im *IntentManager) RejectIntent(id string, reason string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	intent, exists := im.intents[id]
	if !exists {
		return fmt.Errorf("intent not found: %s", id)
	}

	// Pending: human/TTL rejection. Executing: the authorize handler claimed the
	// intent but then aborted (stale-price guard or execution failure) and needs
	// to clean it up and notify the agent. Anything already finalized is gone
	// from the map, so it would have failed the existence check above.
	if intent.Status != IntentStatusPending && intent.Status != IntentStatusExecuting {
		return fmt.Errorf("cannot reject intent in status: %s", intent.Status)
	}

	intent.Status = IntentStatusRejected
	
	if im.onIntentExpiredOrRejected != nil {
		go im.onIntentExpiredOrRejected(intent, reason)
	}

	delete(im.intents, id)
	return nil
}
