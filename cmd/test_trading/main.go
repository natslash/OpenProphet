// Command test_trading is a manual harness for exercising the IBKR services
// against IB Gateway paper (port 4002). It always runs the read paths
// (account/positions/orders); with -place it also places (and by default
// cancels) a single order built from flags.
//
// Examples:
//
//	go run ./cmd/test_trading                                  # reads + default AAPL LMT @10, then cancel
//	go run ./cmd/test_trading -place=false                     # reads only
//	go run ./cmd/test_trading -symbol OESX:20260620:C:5200 -price 50
//	go run ./cmd/test_trading -type ""                         # exercise empty-type rejection
//	go run ./cmd/test_trading -type limit -side sell -qty 1 -price 500
//
// Guardrails: paper only (port 4002), and market orders require -allow-market.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"prophet-trader/interfaces"
	"prophet-trader/services"
	"prophet-trader/tws"
	"strings"
	"time"
)

// simpleWrapper is a no-op wrapper (lifecycle callbacks are not needed here;
// the trading service drives its own confirmation via the dispatcher).
type simpleWrapper struct{ tws.DefaultWrapper }

func (w *simpleWrapper) Error(reqId int, code int, msg string) {
	switch code {
	case 2104, 2106, 2158, 2107: // routine data-farm status notices
	default:
		fmt.Printf("TWS Warning/Error [%d]: %d %s\n", reqId, code, msg)
	}
}

func main() {
	host := flag.String("host", "127.0.0.1", "IB Gateway host")
	port := flag.Int("port", 4002, "IB Gateway port (paper=4002)")
	clientID := flag.Int("client", 6, "API client id")
	doPlace := flag.Bool("place", true, "place an order after the read paths")
	doCancel := flag.Bool("cancel", true, "cancel the placed order")
	symbol := flag.String("symbol", "AAPL", "interface symbol (e.g. AAPL or OESX:20260620:C:5200)")
	side := flag.String("side", "buy", "buy|sell")
	otype := flag.String("type", "LMT", "order type (limit/market/stop or TWS codes); empty exercises rejection")
	qty := flag.Float64("qty", 1.0, "quantity (lots)")
	price := flag.Float64("price", 10.0, "limit price (0 = unset)")
	stop := flag.Float64("stop", 0, "stop/aux price (0 = unset)")
	allowMarket := flag.Bool("allow-market", false, "permit a market order (off by default — can fill)")
	flag.Parse()

	if *doPlace {
		if *port != 4002 {
			fmt.Printf("refusing to place orders on port %d — paper (4002) only\n", *port)
			os.Exit(1)
		}
		if lt := strings.ToLower(strings.TrimSpace(*otype)); (lt == "market" || lt == "mkt") && !*allowMarket {
			fmt.Println("refusing to place a market order without -allow-market (it can fill)")
			os.Exit(1)
		}
	}

	fmt.Printf("=== IBKR manual test (port %d, client %d) ===\n", *port, *clientID)
	client := tws.NewClient(*host, *port, *clientID)
	client.AddWrapper(&simpleWrapper{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fmt.Println("Connecting...")
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()
	time.Sleep(2 * time.Second)
	fmt.Println("Connected.")

	svc := services.NewIBKRTradingService(client)

	// --- reads ---
	withCtx := func(d time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), d)
	}

	fmt.Println("\n--- GetAccount ---")
	c1, x1 := withCtx(6 * time.Second)
	if acc, err := svc.GetAccount(c1); err != nil {
		fmt.Printf("GetAccount error: %v\n", err)
	} else {
		fmt.Printf("Account %s | NetLiq %.2f | Cash %.2f | BuyingPower %.2f\n",
			acc.ID, acc.PortfolioValue, acc.Cash, acc.BuyingPower)
	}
	x1()

	fmt.Println("\n--- GetPositions ---")
	c2, x2 := withCtx(6 * time.Second)
	if ps, err := svc.GetPositions(c2); err != nil {
		fmt.Printf("GetPositions error: %v\n", err)
	} else if len(ps) == 0 {
		fmt.Println("No active positions.")
	} else {
		for _, p := range ps {
			fmt.Printf("- %s: %.2f @ %.2f\n", p.Symbol, p.Qty, p.AvgEntryPrice)
		}
	}
	x2()

	fmt.Println("\n--- ListOrders ---")
	c3, x3 := withCtx(6 * time.Second)
	if os_, err := svc.ListOrders(c3, ""); err != nil {
		fmt.Printf("ListOrders error: %v\n", err)
	} else if len(os_) == 0 {
		fmt.Println("No open orders.")
	} else {
		for _, o := range os_ {
			fmt.Printf("- order %s: %s %.2f %s (%s) - %s\n", o.ID, o.Side, o.Qty, o.Symbol, o.Type, o.Status)
		}
	}
	x3()

	if !*doPlace {
		fmt.Println("\n=== Done (reads only) ===")
		return
	}

	// --- place ---
	fmt.Println("\n--- PlaceOrder ---")
	order := &interfaces.Order{Symbol: *symbol, Qty: *qty, Side: *side, Type: *otype}
	if *price > 0 {
		p := *price
		order.LimitPrice = &p
	}
	if *stop > 0 {
		s := *stop
		order.StopPrice = &s
	}

	c4, x4 := withCtx(8 * time.Second)
	res, err := svc.PlaceOrder(c4, order)
	x4()
	if err != nil {
		fmt.Printf("PlaceOrder error: %v\n", err)
		fmt.Println("\n=== Done ===")
		return
	}
	fmt.Printf("PlaceOrder result: %+v\n", res)
	time.Sleep(3 * time.Second)

	if !*doCancel {
		fmt.Println("\n=== Done (order left open) ===")
		return
	}

	fmt.Println("\n--- CancelOrder ---")
	c5, x5 := withCtx(6 * time.Second)
	if err := svc.CancelOrder(c5, res.OrderID); err != nil {
		fmt.Printf("CancelOrder error: %v\n", err)
	} else {
		fmt.Printf("Cancel sent for %s; waiting for callbacks...\n", res.OrderID)
		time.Sleep(2 * time.Second)
	}
	x5()

	fmt.Println("\n=== Done ===")
}
