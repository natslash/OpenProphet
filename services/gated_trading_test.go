package services

import (
	"context"
	"prophet-trader/interfaces"
	"testing"
	"time"
)

// fakeTrading records whether mutating methods reached the inner service.
type fakeTrading struct {
	placed   int
	canceled int
	optPlaced int
	reads    int
}

func (f *fakeTrading) PlaceOrder(ctx context.Context, o *interfaces.Order) (*interfaces.OrderResult, error) {
	f.placed++
	return &interfaces.OrderResult{OrderID: "X"}, nil
}
func (f *fakeTrading) CancelOrder(ctx context.Context, id string) error { f.canceled++; return nil }
func (f *fakeTrading) PlaceOptionsOrder(ctx context.Context, o *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	f.optPlaced++
	return &interfaces.OrderResult{OrderID: "Y"}, nil
}
func (f *fakeTrading) PlaceComboOrder(ctx context.Context, o *interfaces.ComboOrder) (*interfaces.OrderResult, error) {
	f.optPlaced++
	return &interfaces.OrderResult{OrderID: "Z"}, nil
}
func (f *fakeTrading) GetOrder(ctx context.Context, id string) (*interfaces.Order, error) {
	f.reads++
	return &interfaces.Order{ID: id}, nil
}
func (f *fakeTrading) ListOrders(ctx context.Context, s string) ([]*interfaces.Order, error) {
	f.reads++
	return nil, nil
}
func (f *fakeTrading) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	f.reads++
	return nil, nil
}
func (f *fakeTrading) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	f.reads++
	return &interfaces.Account{}, nil
}
func (f *fakeTrading) GetOptionsChain(ctx context.Context, u string, e time.Time) ([]*interfaces.OptionContract, error) {
	f.reads++
	return nil, nil
}
func (f *fakeTrading) GetOptionsQuote(ctx context.Context, s string) (*interfaces.OptionsQuote, error) {
	f.reads++
	return nil, nil
}
func (f *fakeTrading) GetOptionsPosition(ctx context.Context, s string) (*interfaces.OptionsPosition, error) {
	f.reads++
	return nil, nil
}
func (f *fakeTrading) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	f.reads++
	return nil, nil
}

func TestGatedTrading_DisabledBlocksOrders(t *testing.T) {
	f := &fakeTrading{}
	g := NewGatedTradingService(f, false) // dry-run
	ctx := context.Background()

	if _, err := g.PlaceOrder(ctx, &interfaces.Order{Symbol: "AAPL", Side: "buy", Qty: 1, Type: "limit"}); err == nil {
		t.Error("PlaceOrder should error when disabled")
	}
	if err := g.CancelOrder(ctx, "1"); err == nil {
		t.Error("CancelOrder should error when disabled")
	}
	if _, err := g.PlaceOptionsOrder(ctx, &interfaces.OptionsOrder{}); err == nil {
		t.Error("PlaceOptionsOrder should error when disabled")
	}
	if f.placed+f.canceled+f.optPlaced != 0 {
		t.Errorf("inner mutating methods must not be called when disabled (got placed=%d canceled=%d opt=%d)", f.placed, f.canceled, f.optPlaced)
	}

	// reads still pass through
	_, _ = g.GetAccount(ctx)
	_, _ = g.GetPositions(ctx)
	_, _ = g.ListOrders(ctx, "")
	if f.reads != 3 {
		t.Errorf("reads should pass through; got %d", f.reads)
	}
}

func TestGatedTrading_EnabledPassesThrough(t *testing.T) {
	f := &fakeTrading{}
	g := NewGatedTradingService(f, true)
	ctx := context.Background()

	if _, err := g.PlaceOrder(ctx, &interfaces.Order{Symbol: "AAPL"}); err != nil {
		t.Errorf("PlaceOrder should pass through when enabled: %v", err)
	}
	_ = g.CancelOrder(ctx, "1")
	if f.placed != 1 || f.canceled != 1 {
		t.Errorf("inner should be called when enabled (placed=%d canceled=%d)", f.placed, f.canceled)
	}

	// runtime kill-switch
	g.Disable("test halt")
	if g.Enabled() {
		t.Error("Enabled() should be false after Disable")
	}
	if _, err := g.PlaceOrder(ctx, &interfaces.Order{Symbol: "AAPL"}); err == nil {
		t.Error("PlaceOrder should be blocked after Disable")
	}
	if f.placed != 1 {
		t.Errorf("no further inner placement after Disable; got placed=%d", f.placed)
	}
}
