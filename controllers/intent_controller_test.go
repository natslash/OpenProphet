package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"prophet-trader/config"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ── fakes ──────────────────────────────────────────────────────────────────

// fakeTrading records whether mutating methods reached the broker.
type fakeTrading struct {
	placed    int32
	optPlaced int32
}

func (f *fakeTrading) PlaceOrder(ctx context.Context, o *interfaces.Order) (*interfaces.OrderResult, error) {
	atomic.AddInt32(&f.placed, 1)
	return &interfaces.OrderResult{OrderID: "ENTRY-1", Status: "submitted"}, nil
}
func (f *fakeTrading) CancelOrder(ctx context.Context, id string) error { return nil }
func (f *fakeTrading) PlaceOptionsOrder(ctx context.Context, o *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	atomic.AddInt32(&f.optPlaced, 1)
	return &interfaces.OrderResult{OrderID: "OPT-1", Status: "submitted"}, nil
}
func (f *fakeTrading) GetOrder(ctx context.Context, id string) (*interfaces.Order, error) {
	return &interfaces.Order{ID: id}, nil
}
func (f *fakeTrading) ListOrders(ctx context.Context, s string) ([]*interfaces.Order, error) {
	return nil, nil
}
func (f *fakeTrading) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	return nil, nil
}
func (f *fakeTrading) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	return &interfaces.Account{}, nil
}
func (f *fakeTrading) GetOptionsChain(ctx context.Context, u string, e time.Time) ([]*interfaces.OptionContract, error) {
	return nil, nil
}
func (f *fakeTrading) GetOptionsQuote(ctx context.Context, s string) (*interfaces.OptionsQuote, error) {
	return nil, nil
}
func (f *fakeTrading) GetOptionsPosition(ctx context.Context, s string) (*interfaces.OptionsPosition, error) {
	return nil, nil
}
func (f *fakeTrading) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, nil
}

// fakeData returns a configurable quote (for the stale-price guard / entry price).
type fakeData struct {
	ask float64
	bid float64
}

func (d *fakeData) GetHistoricalBars(ctx context.Context, symbol string, start, end time.Time, tf string) ([]*interfaces.Bar, error) {
	return nil, nil
}
func (d *fakeData) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	return &interfaces.Bar{Symbol: symbol, Close: d.ask}, nil
}
func (d *fakeData) GetLatestQuote(ctx context.Context, symbol string) (*interfaces.Quote, error) {
	return &interfaces.Quote{Symbol: symbol, AskPrice: d.ask, BidPrice: d.bid}, nil
}
func (d *fakeData) GetLatestTrade(ctx context.Context, symbol string) (*interfaces.Trade, error) {
	return &interfaces.Trade{Symbol: symbol, Price: d.ask}, nil
}
func (d *fakeData) StreamBars(ctx context.Context, symbols []string) (<-chan *interfaces.Bar, error) {
	return nil, nil
}

// ── harness ────────────────────────────────────────────────────────────────

type intentHarness struct {
	router  *gin.Engine
	im      *services.IntentManager
	trading *fakeTrading
	gated   *services.GatedTradingService
}

func newIntentHarness(t *testing.T, adminToken string, tradingEnabled bool, quoteAsk float64) *intentHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	config.AppConfig = &config.Config{
		AdminToken:              adminToken,
		RequireDoubleConfirm:    true,
		MaxPriceSlippagePercent: 0.5,
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	fake := &fakeTrading{}
	gated := services.NewGatedTradingService(fake, tradingEnabled)
	data := &fakeData{ask: quoteAsk, bid: quoteAsk}

	storage, err := database.NewLocalStorage("file::memory:?cache=shared&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	pm := services.NewPositionManager(gated, data, storage)
	im := services.NewIntentManager(300, logger)

	ic := NewIntentController(im, pm, gated, data, nil)

	r := gin.New()
	r.GET("/api/v1/beat/intents", ic.HandleGetIntents)
	r.POST("/api/v1/beat/authorize/:id", ic.HandleAuthorizeIntent)
	r.POST("/api/v1/beat/reject/:id", ic.HandleRejectIntent)

	return &intentHarness{router: r, im: im, trading: fake, gated: gated}
}

func (h *intentHarness) do(method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func optionsIntentPayload() []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"symbol": "ESTX50:20260619:C:5000", "action": "buy", "qty": 1, "order_type": "MKT",
	})
	return b
}

func managedIntentPayload() []byte {
	entry, sl, tp := 5000.0, 4900.0, 5200.0
	b, _ := json.Marshal(services.PlaceManagedPositionRequest{
		Symbol:          "ESTX50",
		Side:            "buy",
		EntryStrategy:   "limit",
		EntryPrice:      &entry,
		StopLossPrice:   &sl,
		TakeProfitPrice: &tp,
		ExplicitQuantity: func() *int { q := 1; return &q }(),
	})
	return b
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestIntentController_AuthorizeRequiresValidToken(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	// No token → 401.
	if w := h.do("POST", "/api/v1/beat/authorize/"+id, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", w.Code)
	}
	// Wrong token → 401.
	if w := h.do("POST", "/api/v1/beat/authorize/"+id, "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", w.Code)
	}
	// Order must NOT have been placed by the rejected attempts.
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 0 {
		t.Errorf("order placed despite auth failure (optPlaced=%d)", n)
	}
	// Intent must still be pending in the queue.
	if _, err := h.im.GetIntent(id); err != nil {
		t.Errorf("intent should remain after failed auth: %v", err)
	}
}

func TestIntentController_FailsClosedWhenNoAdminToken(t *testing.T) {
	h := newIntentHarness(t, "", true, 5000) // no admin token configured
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	if w := h.do("POST", "/api/v1/beat/authorize/"+id, "anything"); w.Code != http.StatusForbidden {
		t.Errorf("no admin token configured: got %d, want 403", w.Code)
	}
}

func TestIntentController_GetIntents(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	w := h.do("GET", "/api/v1/beat/intents", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GetIntents: got %d, want 200", w.Code)
	}
	var got []services.Intent
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal intents: %v", err)
	}
	if len(got) != 1 || got[0].Symbol != "ESTX50" {
		t.Errorf("unexpected intents payload: %+v", got)
	}
}

func TestIntentController_AuthorizeOptionsOrder_Success(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	// CurrentPrice=0 skips the stale-price guard so we test the execution path.
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusOK {
		t.Fatalf("authorize: got %d, body=%s", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 1 {
		t.Errorf("options order should reach broker once, got %d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("authorized intent should be removed from the queue")
	}
}

func TestIntentController_AuthorizeManagedPosition_Success(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	id, _ := h.im.CreateIntent(services.IntentTypeManagedPosition, managedIntentPayload(), "ESTX50", "buy", 1, 0)

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusOK {
		t.Fatalf("authorize: got %d, body=%s", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.placed); n != 1 {
		t.Errorf("managed entry order should reach broker once, got %d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("authorized intent should be removed from the queue")
	}
}

// The kill-switch (GatedTradingService) must be authoritative at *execution*
// time, not just at intent-creation time. An intent queued while trading is
// enabled but authorized after a halt must NOT reach the broker, and must be
// cleaned out of the queue.
func TestIntentController_KillSwitchBlocksAuthorizedOrder(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	h.gated.Disable("test halt") // e.g. broker disconnect after the intent was queued

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("authorize during halt: got %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 0 {
		t.Errorf("order must not reach broker during halt, got optPlaced=%d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("failed intent should be cleaned out of the queue (not leaked as EXECUTING)")
	}
}

// A stale intent (live price drifted beyond MaxPriceSlippagePercent) must be
// rejected without reaching the broker, and removed from the queue.
func TestIntentController_StalePriceGuardRejects(t *testing.T) {
	// Intent priced at 5000; live quote at 6000 → 20% drift >> 0.5% max.
	h := newIntentHarness(t, "secret", true, 6000)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 5000)

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("stale authorize: got %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 0 {
		t.Errorf("stale order must not reach broker, got optPlaced=%d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("stale intent should be removed from the queue")
	}
}

// A small drift within MaxPriceSlippagePercent must pass the guard and execute.
func TestIntentController_StalePriceGuardAllowsSmallDrift(t *testing.T) {
	// Intent at 5000, live at 5010 → 0.2% drift < 0.5% max.
	h := newIntentHarness(t, "secret", true, 5010)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 5000)

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusOK {
		t.Fatalf("small-drift authorize: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 1 {
		t.Errorf("order should execute within slippage, got optPlaced=%d", n)
	}
}

// On the live port, an intent with no reference price (quote was unavailable at
// creation) cannot be authorized — slippage can't be validated, so fail closed.
func TestIntentController_LiveRequiresReferencePrice(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	config.AppConfig.IBKRPort = 4001 // simulate live (harness resets config per test)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0) // CurrentPrice=0

	w := h.do("POST", "/api/v1/beat/authorize/"+id, "secret")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("live authorize without reference price: got %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 0 {
		t.Errorf("order must not reach broker without price validation on live, got %d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("rejected intent should be removed from the queue")
	}
}

// On paper, the same no-reference-price intent is allowed (so off-hours testing
// isn't blocked). Covered by TestIntentController_AuthorizeOptionsOrder_Success,
// which authorizes a CurrentPrice=0 intent on the default (non-live) port.

func TestIntentController_Reject(t *testing.T) {
	h := newIntentHarness(t, "secret", true, 5000)
	id, _ := h.im.CreateIntent(services.IntentTypeOptionsOrder, optionsIntentPayload(), "ESTX50", "buy", 1, 0)

	// Reject requires the admin token.
	if w := h.do("POST", "/api/v1/beat/reject/"+id, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("reject without token: got %d, want 401", w.Code)
	}

	w := h.do("POST", "/api/v1/beat/reject/"+id, "secret")
	if w.Code != http.StatusOK {
		t.Fatalf("reject: got %d, body=%s", w.Code, w.Body.String())
	}
	if n := atomic.LoadInt32(&h.trading.optPlaced); n != 0 {
		t.Errorf("rejected intent must not place an order, got %d", n)
	}
	if _, err := h.im.GetIntent(id); err == nil {
		t.Error("rejected intent should be removed from the queue")
	}
}
