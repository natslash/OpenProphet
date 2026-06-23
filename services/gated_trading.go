package services

import (
	"context"
	"fmt"
	"log"
	"prophet-trader/interfaces"
	"sync/atomic"
	"time"
)

// GatedTradingService wraps a TradingService with a master kill-switch.
//
// When trading is disabled (the default), the order-mutating methods
// (PlaceOrder, CancelOrder, PlaceOptionsOrder) log the full intent and return
// an error WITHOUT touching the broker — "dry-run" mode. Read-only methods
// always pass through. Trading must be explicitly enabled (Phase 4.3e), and can
// be turned off at runtime via Disable() (e.g. on a broker disconnect → halt).
type GatedTradingService struct {
	inner   interfaces.TradingService
	enabled atomic.Bool
}

var _ interfaces.TradingService = (*GatedTradingService)(nil)

// NewGatedTradingService wraps inner. enabled=false means dry-run (no orders).
func NewGatedTradingService(inner interfaces.TradingService, enabled bool) *GatedTradingService {
	g := &GatedTradingService{inner: inner}
	g.enabled.Store(enabled)
	if enabled {
		log.Printf("[GATE] trading ENABLED — orders will be sent to the broker")
	} else {
		log.Printf("[GATE] trading DISABLED (dry-run) — order intent will be logged but not sent")
	}
	return g
}

// Enabled reports whether live order placement is currently allowed.
func (g *GatedTradingService) Enabled() bool { return g.enabled.Load() }

// Disable turns off order placement at runtime (e.g. broker disconnect → halt).
func (g *GatedTradingService) Disable(reason string) {
	if g.enabled.Swap(false) {
		log.Printf("[GATE] trading DISABLED at runtime: %s", reason)
	}
}

// Enable turns on order placement at runtime (e.g. after successful reconnect).
func (g *GatedTradingService) Enable(reason string) {
	if !g.enabled.Swap(true) {
		log.Printf("[GATE] trading ENABLED at runtime: %s", reason)
	}
}

// blocked logs the dry-run intent and returns the standard refusal error.
func (g *GatedTradingService) blocked(action string) error {
	log.Printf("[GATE][DRY-RUN] %s suppressed — trading disabled; no order sent", action)
	return fmt.Errorf("trading disabled (dry-run mode): %s not sent", action)
}

func (g *GatedTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	if !g.enabled.Load() {
		return nil, g.blocked(fmt.Sprintf("PlaceOrder %s %s qty=%v type=%s", order.Side, order.Symbol, order.Qty, order.Type))
	}
	return g.inner.PlaceOrder(ctx, order)
}

func (g *GatedTradingService) CancelOrder(ctx context.Context, orderID string) error {
	if !g.enabled.Load() {
		return g.blocked("CancelOrder " + orderID)
	}
	return g.inner.CancelOrder(ctx, orderID)
}

func (g *GatedTradingService) PlaceOptionsOrder(ctx context.Context, order *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	if !g.enabled.Load() {
		return nil, g.blocked("PlaceOptionsOrder")
	}
	return g.inner.PlaceOptionsOrder(ctx, order)
}

// --- read-only pass-throughs ---

func (g *GatedTradingService) GetOrder(ctx context.Context, orderID string) (*interfaces.Order, error) {
	return g.inner.GetOrder(ctx, orderID)
}

func (g *GatedTradingService) ListOrders(ctx context.Context, status string) ([]*interfaces.Order, error) {
	return g.inner.ListOrders(ctx, status)
}

func (g *GatedTradingService) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	return g.inner.GetPositions(ctx)
}

func (g *GatedTradingService) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	return g.inner.GetAccount(ctx)
}

func (g *GatedTradingService) GetOptionsChain(ctx context.Context, underlying string, expiration time.Time) ([]*interfaces.OptionContract, error) {
	return g.inner.GetOptionsChain(ctx, underlying, expiration)
}

func (g *GatedTradingService) GetOptionsQuote(ctx context.Context, symbol string) (*interfaces.OptionsQuote, error) {
	return g.inner.GetOptionsQuote(ctx, symbol)
}

func (g *GatedTradingService) GetOptionsPosition(ctx context.Context, symbol string) (*interfaces.OptionsPosition, error) {
	return g.inner.GetOptionsPosition(ctx, symbol)
}

func (g *GatedTradingService) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return g.inner.ListOptionsPositions(ctx)
}
