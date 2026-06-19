package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"prophet-trader/interfaces"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TradeIntent is a proposed-but-unauthorised trade produced by the beat. It is
// NOT executed until a human authorises it (token-gated endpoint). TP/SL are
// stored as percents so the entry/exit prices are re-derived from a fresh price
// at authorisation time (avoids acting on a stale snapshot).
type TradeIntent struct {
	ID                string    `json:"id"`
	Symbol            string    `json:"symbol"`
	Side              string    `json:"side"`
	Quantity          int       `json:"quantity"`
	EntryPrice        float64   `json:"entry_price"` // far-from-market limit (won't fill by accident)
	StopLossPercent   float64   `json:"stop_loss_percent"`
	TakeProfitPercent float64   `json:"take_profit_percent"`
	Reason            string    `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// AutonomousBeatConfig configures the supervised beat.
type AutonomousBeatConfig struct {
	Symbol             string        // configured target contract (e.g. ESTX50:20260619:C:6325)
	Interval           time.Duration // tick + intent TTL
	MaxDailyExecutions int           // hard cap on authorised executions per day
	ForceSignal        bool          // testing aid: emit a buy signal every tick
}

// AutonomousBeat periodically proposes trade intents for a configured contract.
// It never executes anything itself: intents are stored and require an explicit,
// token-authorised call to Authorize() (which routes through the gated
// PositionManager). Hard caps: 1 lot per intent, MaxDailyExecutions per day.
type AutonomousBeat struct {
	data   interfaces.DataService
	pm     *PositionManager
	logger *logrus.Logger
	cfg    AutonomousBeatConfig

	mu          sync.Mutex
	intents     map[string]*TradeIntent
	executedDay string
	executed    int
}

func NewAutonomousBeat(data interfaces.DataService, pm *PositionManager, logger *logrus.Logger, cfg AutonomousBeatConfig) *AutonomousBeat {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.MaxDailyExecutions <= 0 {
		cfg.MaxDailyExecutions = 3
	}
	return &AutonomousBeat{
		data:    data,
		pm:      pm,
		logger:  logger,
		cfg:     cfg,
		intents: make(map[string]*TradeIntent),
	}
}

// Run ticks until ctx is cancelled, proposing intents (never executing).
func (b *AutonomousBeat) Run(ctx context.Context) {
	t := time.NewTicker(b.cfg.Interval)
	defer t.Stop()
	b.logger.WithFields(logrus.Fields{
		"symbol": b.cfg.Symbol, "interval": b.cfg.Interval, "max_daily": b.cfg.MaxDailyExecutions,
	}).Warn("[BEAT] supervised autonomous beat started — intents require human authorization to execute")
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b.tick(ctx)
		}
	}
}

func (b *AutonomousBeat) tick(ctx context.Context) {
	b.expireIntents()
	if !b.canExecuteToday() {
		b.logger.Warn("[BEAT] daily execution cap reached; not proposing further intents today")
		return
	}
	side, entry, reason, ok := b.signal(ctx)
	if !ok {
		return
	}
	intent := &TradeIntent{
		ID:                randomID(),
		Symbol:            b.cfg.Symbol,
		Side:              side,
		Quantity:          1, // hard cap
		EntryPrice:        entry,
		StopLossPercent:   20,
		TakeProfitPercent: 20,
		Reason:            reason,
		CreatedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(b.cfg.Interval),
	}
	b.mu.Lock()
	b.intents[intent.ID] = intent
	b.mu.Unlock()
	b.logger.WithFields(logrus.Fields{
		"intent_id": intent.ID, "symbol": intent.Symbol, "side": intent.Side,
		"qty": intent.Quantity, "entry": intent.EntryPrice, "expires_at": intent.ExpiresAt.Format(time.RFC3339),
	}).Warn("[BEAT] TRADE INTENT GENERATED — human authorization required: POST /api/v1/beat/authorize/<id> (admin token)")
}

// signal fetches recent bars and decides buy/hold. This is a placeholder
// strategy — 4.3e validates the human-in-the-loop machinery, not alpha.
func (b *AutonomousBeat) signal(ctx context.Context) (side string, entry float64, reason string, ok bool) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	bars, err := b.data.GetHistoricalBars(c, b.cfg.Symbol, time.Now().Add(-2*time.Hour), time.Now(), "5Min")
	if err != nil || len(bars) == 0 {
		b.logger.WithError(err).Warn("[BEAT] no data for signal; skipping")
		return "", 0, "", false
	}
	last := bars[len(bars)-1].Close

	buy := b.cfg.ForceSignal
	reason = "forced buy (test mode)"
	if !b.cfg.ForceSignal {
		if bars[len(bars)-1].Close > bars[0].Close {
			buy, reason = true, "momentum up (last close > window start)"
		}
	}
	if !buy {
		b.logger.Debug("[BEAT] no buy signal this tick")
		return "", 0, "", false
	}
	// Far-from-market limit so the resting bracket cannot fill by accident
	// (guardrail: test small, far from market).
	return "buy", last * 0.5, reason, true
}

// ListIntents returns the currently-pending (non-expired) intents.
func (b *AutonomousBeat) ListIntents() []*TradeIntent {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	out := make([]*TradeIntent, 0, len(b.intents))
	for _, it := range b.intents {
		if now.Before(it.ExpiresAt) {
			out = append(out, it)
		}
	}
	return out
}

// Authorize executes a pending intent (human-gated by the controller). It
// enforces the daily cap, one-shot consumes the intent, and routes through the
// gated PositionManager (so nothing executes unless TradingEnabled=true).
func (b *AutonomousBeat) Authorize(ctx context.Context, id string) (*ManagedPosition, error) {
	b.mu.Lock()
	b.rollDay()
	if b.executed >= b.cfg.MaxDailyExecutions {
		b.mu.Unlock()
		return nil, fmt.Errorf("daily execution cap reached (%d)", b.cfg.MaxDailyExecutions)
	}
	intent, ok := b.intents[id]
	if !ok {
		b.mu.Unlock()
		return nil, fmt.Errorf("intent %q not found", id)
	}
	if time.Now().After(intent.ExpiresAt) {
		delete(b.intents, id)
		b.mu.Unlock()
		return nil, fmt.Errorf("intent %q expired", id)
	}
	delete(b.intents, id) // one-shot: consume before placement
	b.mu.Unlock()

	entry, slp, tpp, qty := intent.EntryPrice, intent.StopLossPercent, intent.TakeProfitPercent, intent.Quantity
	req := &PlaceManagedPositionRequest{
		Symbol:            intent.Symbol,
		Side:              intent.Side,
		Strategy:          "DAY_TRADE",
		ExplicitQuantity:  &qty,
		EntryStrategy:     "limit",
		EntryPrice:        &entry,
		StopLossPercent:   &slp,
		TakeProfitPercent: &tpp,
		Notes:             "autonomous beat (human-authorized): " + intent.Reason,
	}
	pos, err := b.pm.PlaceManagedPosition(ctx, req)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.executed++
	executedToday := b.executed
	b.mu.Unlock()
	b.logger.WithFields(logrus.Fields{
		"intent_id": id, "position_id": pos.ID, "executed_today": executedToday,
	}).Warn("[BEAT] intent AUTHORIZED and executed")
	return pos, nil
}

func (b *AutonomousBeat) expireIntents() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for id, it := range b.intents {
		if now.After(it.ExpiresAt) {
			delete(b.intents, id)
		}
	}
}

func (b *AutonomousBeat) canExecuteToday() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollDay()
	return b.executed < b.cfg.MaxDailyExecutions
}

// rollDay resets the execution counter when the date changes. Caller holds mu.
func (b *AutonomousBeat) rollDay() {
	today := time.Now().Format("2006-01-02")
	if b.executedDay != today {
		b.executedDay = today
		b.executed = 0
	}
}

func randomID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("intent_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
