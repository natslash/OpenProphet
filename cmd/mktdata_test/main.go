package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/tws"
	"time"

	"github.com/shopspring/decimal"
)

var tickNames = map[int]string{
	0: "BidSize", 1: "Bid", 2: "Ask", 3: "AskSize",
	4: "Last", 5: "LastSize", 6: "High", 7: "Low", 8: "Volume", 9: "Close",
	14: "Open", 45: "LastTimestamp",
	66: "DelayedBid", 67: "DelayedAsk", 68: "DelayedLast",
	69: "DelayedBidSize", 70: "DelayedAskSize", 71: "DelayedLastSize",
	72: "DelayedHigh", 73: "DelayedLow", 75: "DelayedClose",
	76: "DelayedOpen",
}

func tn(t int) string {
	if n, ok := tickNames[t]; ok {
		return fmt.Sprintf("%s(%d)", n, t)
	}
	return fmt.Sprintf("Type(%d)", t)
}

type liveWrapper struct {
	tws.DefaultWrapper
}

func (w *liveWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {
	fmt.Printf("[%s] PRICE  reqId=%-4d %-20s  price=%-12.4f  size=%s\n",
		time.Now().Format("15:04:05"), reqId, tn(tickType), price, size.String())
}

func (w *liveWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	fmt.Printf("[%s] SIZE   reqId=%-4d %-20s  size=%s\n",
		time.Now().Format("15:04:05"), reqId, tn(tickType), size.String())
}

func (w *liveWrapper) TickString(reqId int64, tickType int, value string) {
	fmt.Printf("[%s] STRING reqId=%-4d %-20s  value=%s\n",
		time.Now().Format("15:04:05"), reqId, tn(tickType), value)
}

func (w *liveWrapper) TickGeneric(reqId int64, tickType int, value float64) {
	fmt.Printf("[%s] GENERIC reqId=%-4d %-20s  value=%.4f\n",
		time.Now().Format("15:04:05"), reqId, tn(tickType), value)
}

func (w *liveWrapper) Error(reqId int, code int, msg string) {
	switch code {
	case 2104, 2106, 2158, 2107, 2119:
		return
	default:
		fmt.Printf("[%s] ERROR  reqId=%-4d code=%-6d %s\n",
			time.Now().Format("15:04:05"), reqId, code, msg)
	}
}

func main() {
	mode := "option"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var contract tws.Contract
	var label string
	duration := 15 * time.Second

	switch mode {
	case "index":
		contract = tws.Contract{
			Symbol:   "ESTX50",
			SecType:  "IND",
			Exchange: "EUREX",
			Currency: "EUR",
		}
		label = "ESTX50 INDEX"
	case "option":
		expiry := "20260918"
		strike := 4500.0
		right := "C"
		if len(os.Args) > 2 { expiry = os.Args[2] }
		if len(os.Args) > 3 { fmt.Sscanf(os.Args[3], "%f", &strike) }
		if len(os.Args) > 4 { right = os.Args[4] }
		contract = tws.Contract{
			Symbol:                       "ESTX50",
			SecType:                      "OPT",
			Exchange:                     "EUREX",
			Currency:                     "EUR",
			Multiplier:                   "10",
			LastTradeDateOrContractMonth: expiry,
			Strike:                       strike,
			Right:                        right,
		}
		label = fmt.Sprintf("ESTX50 %s %.0f%s OPT", expiry, strike, right)
	case "stock":
		sym := "AAPL"
		if len(os.Args) > 2 { sym = os.Args[2] }
		contract = tws.Contract{
			Symbol:   sym,
			SecType:  "STK",
			Exchange: "SMART",
			Currency: "USD",
		}
		label = sym + " STK"
	default:
		fmt.Printf("Usage: %s [index|option|stock] [args...]\n", os.Args[0])
		fmt.Println("  index")
		fmt.Println("  option [expiry] [strike] [C|P]")
		fmt.Println("  stock [symbol]")
		os.Exit(1)
	}

	fmt.Printf("=== Market Data Test: %s ===\n", label)
	fmt.Printf("Streaming for %s...\n\n", duration)

	client := tws.NewClient("127.0.0.1", 4002, 99)
	wrapper := &liveWrapper{}
	client.AddWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	time.Sleep(1 * time.Second)

	client.Encoder().ReqMarketDataType(4)
	fmt.Printf("[%s] Connected. MarketDataType=4. Subscribing to %s...\n\n", time.Now().Format("15:04:05"), label)

	reqId := client.NextOrderId()
	if err := client.Encoder().ReqMktData(reqId, contract, "", false, false); err != nil {
		fmt.Printf("FAIL: ReqMktData error: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(duration)

	client.Encoder().CancelMktData(reqId)
	time.Sleep(500 * time.Millisecond)
	fmt.Println("\nDone.")
}
