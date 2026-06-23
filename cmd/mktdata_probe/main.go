// Command mktdata_probe checks which market data subscriptions are active by
// requesting data for a set of instruments and observing which return ticks vs
// subscription errors (code 354) or "no security definition" (code 200).
//
// Usage:
//
//	go run ./cmd/mktdata_probe
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"prophet-trader/tws"

	"github.com/shopspring/decimal"
)

type probeResult struct {
	label    string
	gotData  bool
	errCode  int
	errMsg   string
}

type probeWrapper struct {
	tws.DefaultWrapper
	mu      sync.Mutex
	results map[int64]*probeResult
}

func (w *probeWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r, ok := w.results[reqId]; ok {
		r.gotData = true
	}
}

func (w *probeWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r, ok := w.results[reqId]; ok {
		r.gotData = true
	}
}

func (w *probeWrapper) TickGeneric(reqId int64, tickType int, value float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r, ok := w.results[reqId]; ok {
		r.gotData = true
	}
}

func (w *probeWrapper) Error(reqId int, code int, msg string) {
	if code >= 2100 && code <= 2200 || code >= 1100 && code <= 1102 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if r, ok := w.results[int64(reqId)]; ok {
		r.errCode = code
		r.errMsg = msg
	}
}

type instrument struct {
	label    string
	contract tws.Contract
}

func main() {
	instruments := []instrument{
		{"ESTX50 Index (EUREX)", tws.Contract{Symbol: "ESTX50", SecType: "IND", Exchange: "EUREX", Currency: "EUR"}},
		{"ESTX50 Option OESX (EUREX)", tws.Contract{Symbol: "ESTX50", SecType: "OPT", Exchange: "EUREX", Currency: "EUR", Multiplier: "10", TradingClass: "OESX", LastTradeDateOrContractMonth: "20260821", Strike: 6000, Right: "P"}},
		{"DAX Index (EUREX)", tws.Contract{Symbol: "DAX", SecType: "IND", Exchange: "EUREX", Currency: "EUR"}},
		{"SPX Index (CBOE)", tws.Contract{Symbol: "SPX", SecType: "IND", Exchange: "CBOE", Currency: "USD"}},
		{"SPX Option (CBOE)", tws.Contract{Symbol: "SPX", SecType: "OPT", Exchange: "CBOE", Currency: "USD", Multiplier: "100", LastTradeDateOrContractMonth: "20260821", Strike: 5500, Right: "P"}},
		{"NDX Index (NASDAQ)", tws.Contract{Symbol: "NDX", SecType: "IND", Exchange: "NASDAQ", Currency: "USD"}},
		{"AAPL Stock (SMART/USD)", tws.Contract{Symbol: "AAPL", SecType: "STK", Exchange: "SMART", Currency: "USD"}},
		{"MSFT Stock (SMART/USD)", tws.Contract{Symbol: "MSFT", SecType: "STK", Exchange: "SMART", Currency: "USD"}},
		{"SAP Stock (SMART/EUR)", tws.Contract{Symbol: "SAP", SecType: "STK", Exchange: "SMART", Currency: "EUR"}},
		{"SIE Stock (SMART/EUR)", tws.Contract{Symbol: "SIE", SecType: "STK", Exchange: "SMART", Currency: "EUR"}},
		{"EURUSD Forex", tws.Contract{Symbol: "EUR", SecType: "CASH", Exchange: "IDEALPRO", Currency: "USD"}},
		{"ES Future (CME)", tws.Contract{Symbol: "ES", SecType: "FUT", Exchange: "CME", Currency: "USD", LastTradeDateOrContractMonth: "202609"}},
		{"FESX Future (EUREX)", tws.Contract{Symbol: "ESTX50", SecType: "FUT", Exchange: "EUREX", Currency: "EUR", LastTradeDateOrContractMonth: "202609"}},
	}

	fmt.Println("=== IB Market Data Subscription Probe ===")
	fmt.Printf("Testing %d instruments against IB Gateway paper (4002)...\n\n", len(instruments))

	client := tws.NewClient("127.0.0.1", 4002, 98)
	pw := &probeWrapper{results: make(map[int64]*probeResult)}
	client.AddWrapper(pw)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	time.Sleep(500 * time.Millisecond)

	// Request delayed data (type 4) so we see something even without real-time subs
	client.Encoder().ReqMarketDataType(3) // 3 = delayed-frozen (falls back gracefully)

	var reqIds []int64
	for _, inst := range instruments {
		reqId := client.NextOrderId()
		reqIds = append(reqIds, reqId)
		pw.mu.Lock()
		pw.results[reqId] = &probeResult{label: inst.label}
		pw.mu.Unlock()

		if err := client.Encoder().ReqMktData(reqId, inst.contract, "", false, false); err != nil {
			fmt.Printf("  %-35s  SEND ERROR: %v\n", inst.label, err)
			continue
		}
	}

	// Wait for responses
	fmt.Println("Waiting 8 seconds for responses...")
	time.Sleep(8 * time.Second)

	// Cancel all subscriptions
	for _, reqId := range reqIds {
		client.Encoder().CancelMktData(reqId)
	}
	time.Sleep(300 * time.Millisecond)

	// Report
	fmt.Printf("%-40s  %-12s  %s\n", "INSTRUMENT", "STATUS", "DETAIL")
	fmt.Println(repeat("-", 90))

	for _, reqId := range reqIds {
		pw.mu.Lock()
		r := pw.results[reqId]
		pw.mu.Unlock()

		status := "NO DATA"
		detail := "no ticks received"
		if r.gotData {
			status = "SUBSCRIBED"
			detail = "ticks received OK"
		}
		if r.errCode > 0 {
			switch r.errCode {
			case 354:
				status = "NO SUB"
				detail = "not subscribed to market data"
			case 200:
				status = "NO DEF"
				detail = "security definition not found"
			case 10167:
				status = "DELAYED OK"
				detail = "delayed data available"
			default:
				status = fmt.Sprintf("ERR %d", r.errCode)
				detail = r.errMsg
			}
			if r.gotData {
				status = "SUBSCRIBED"
				detail = fmt.Sprintf("ticks received (also got code %d)", r.errCode)
			}
		}

		fmt.Printf("%-40s  %-12s  %s\n", r.label, status, detail)
	}
	fmt.Println()
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
