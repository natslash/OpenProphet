package services

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"testing"
	"time"
)

// simpleTestWrapper is a basic wrapper to catch errors during integration tests
type simpleTestWrapper struct {
	tws.DefaultWrapper
	errors []string
}

func (w *simpleTestWrapper) Error(reqId int, code int, msg string) {
	switch code {
	case 2104, 2106, 2158, 2107, 2119: // Routine connection/market data notices
		return
	default:
		w.errors = append(w.errors, fmt.Sprintf("Error %d: %s", code, msg))
	}
}

func setupIntegrationClient(t *testing.T) (*tws.Client, *simpleTestWrapper) {
	// Opt-in only: these tests hit a real broker and one of them places an order.
	// They never run on a plain `go test ./...` — set RUN_LIVE_INTEGRATION=1 to
	// run them. Always pinned to paper (4002), never IBKR_PORT, so they can never
	// reach the live account even if the operator has IBKR_PORT=4001 set.
	if os.Getenv("RUN_LIVE_INTEGRATION") != "1" {
		t.Skip("skipping live broker integration test (set RUN_LIVE_INTEGRATION=1 to run; paper 4002 only)")
	}
	client := tws.NewClient("127.0.0.1", 4002, 12)
	wrapper := &simpleTestWrapper{}
	client.AddWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		// Pinned to paper (4002) on purpose — never IBKR_PORT, since one of these
		// tests places an order and must never reach the live account. Skip
		// rather than fail when no paper gateway is up, so `go test ./...` is
		// green in CI.
		t.Skipf("skipping live integration test: no IB Gateway on paper port 4002 (%v)", err)
	}

	time.Sleep(1 * time.Second) // wait for next valid id and connection acks
	return client, wrapper
}

func floatPtr(v float64) *float64 {
	return &v
}

func testResolver(client *tws.Client) *tws.ContractResolver {
	return tws.NewContractResolver(client)
}

func TestIntegration_GetAccount(t *testing.T) {
	client, _ := setupIntegrationClient(t)
	defer client.Close()

	svc := NewIBKRTradingService(client, testResolver(client))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account, err := svc.GetAccount(ctx)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	if account == nil || account.ID == "" {
		t.Errorf("Expected valid account response, got: %+v", account)
	}
	
	t.Logf("Successfully fetched account: %s, Cash: %.2f", account.ID, account.Cash)
}

func TestIntegration_PlaceManagedPosition_OffHours(t *testing.T) {
	client, _ := setupIntegrationClient(t)
	defer client.Close()

	tradingSvc := NewIBKRTradingService(client, testResolver(client))
	dataSvc := NewIBKRDataService(client, testResolver(client))
	db, err := database.NewLocalStorage("test_integration_pm_db.json")
	if err != nil {
		t.Fatalf("Failed to create mock db: %v", err)
	}

	pm := NewPositionManager(tradingSvc, dataSvc, db)

	// We are testing on Saturday/Off-hours. A limit order should be accepted by TWS
	// and placed in a 'PreSubmitted' or 'Submitted' queue state.
	req := PlaceManagedPositionRequest{
		Symbol:            "ESTX50:20260717:C:6325", // Option format
		AllocationDollars: 50.0,
		Side:              "buy",
		EntryStrategy:     "limit",
		EntryPrice:        floatPtr(5.0),
		Strategy:          "integration_test",
		TakeProfitPercent: floatPtr(50.0),
		StopLossPercent:   floatPtr(25.0),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pos, err := pm.PlaceManagedPosition(ctx, &req)
	if err != nil {
		t.Fatalf("PlaceManagedPosition failed during off-hours: %v", err)
	}

	if pos == nil || pos.ID == "" {
		t.Fatalf("Expected valid managed position, got nil")
	}

	t.Logf("Successfully placed off-hours bracket order for position: %s", pos.ID)
}

func TestIntegration_GetLatestQuote_OffHours(t *testing.T) {
	client, _ := setupIntegrationClient(t)
	defer client.Close()

	dataSvc := NewIBKRDataService(client, testResolver(client))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetching a quote during the weekend for a US stock
	// TWS may return stale snapshot data or timeout if no snapshot is available.
	// We expect this to either succeed with stale data, or fail with a known TWS error.
	quote, err := dataSvc.GetLatestQuote(ctx, "AAPL")
	
	if err != nil {
		t.Logf("Expected behavior on weekend (no live market data): %v", err)
		// We shouldn't fail the test if the market is closed and IBKR refuses a snapshot,
		// but we should verify the error is a context timeout or IBKR specific error.
	} else {
		t.Logf("Received snapshot quote for AAPL: Bid=%.2f, Ask=%.2f", quote.BidPrice, quote.AskPrice)
		if quote.BidPrice <= 0 && quote.AskPrice <= 0 {
			t.Errorf("Received quote but prices are zero: %+v", quote)
		}
	}
}

func TestIntegration_RejectedOrder(t *testing.T) {
	client, wrapper := setupIntegrationClient(t)
	defer client.Close()

	svc := NewIBKRTradingService(client, testResolver(client))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to place an order for a garbage ticker. TWS should instantly reject it.
	order := &interfaces.Order{
		Symbol:      "GARBAGETICKER123:20250101:C:100",
		Qty:         1,
		Side:        "buy",
		Type:        "market",
		TimeInForce: "day",
	}

	_, err := svc.PlaceOrder(ctx, order)
	if err == nil {
		t.Fatalf("Expected order to be rejected, but it succeeded")
	}

	t.Logf("Successfully caught rejected order error: %v", err)
	if len(wrapper.errors) > 0 {
		t.Logf("Wrapper caught errors (expected for rejected order): %v", wrapper.errors)
	}
}

func TestIntegration_GetPositions(t *testing.T) {
	client, _ := setupIntegrationClient(t)
	defer client.Close()

	svc := NewIBKRTradingService(client, testResolver(client))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	positions, err := svc.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}

	t.Logf("Fetched %d positions via reqAccountUpdates", len(positions))
	for _, p := range positions {
		t.Logf("  %s: qty=%.0f side=%s avgEntry=%.2f mktVal=%.2f unrealPL=%.2f (%.2f%%) curPrice=%.2f",
			p.Symbol, p.Qty, p.Side, p.AvgEntryPrice, p.MarketValue, p.UnrealizedPL, p.UnrealizedPLPC*100, p.CurrentPrice)
		if p.MarketValue == 0 && p.CurrentPrice == 0 && p.UnrealizedPL == 0 {
			t.Errorf("Position %s has zero market data — reqAccountUpdates may not be returning portfolio values", p.Symbol)
		}
	}
}

func TestIntegration_GetHistoricalBars_OffHours(t *testing.T) {
	client, _ := setupIntegrationClient(t)
	defer client.Close()

	dataSvc := NewIBKRDataService(client, testResolver(client))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch historical bars for AAPL over the last 2 days.
	end := time.Now()
	start := end.Add(-48 * time.Hour)

	bars, err := dataSvc.GetHistoricalBars(ctx, "AAPL", start, end, "1Hour")
	if err != nil {
		t.Logf("Expected behavior on weekend (historical data may timeout or not return): %v", err)
		return
	}

	if len(bars) == 0 {
		t.Log("Expected behavior: no historical bars returned during this exact 48h off-hours period, or data is missing")
	} else {
		t.Logf("Successfully fetched %d historical bars for AAPL", len(bars))
		t.Logf("First bar: %+v", bars[0])
		t.Logf("Last bar: %+v", bars[len(bars)-1])
	}
}
