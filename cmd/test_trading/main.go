package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/interfaces"
	"prophet-trader/services"
	"prophet-trader/tws"
	"time"
	"github.com/shopspring/decimal"
)

// simpleWrapper just prints errors
type simpleWrapper struct{}
func (w *simpleWrapper) NextValidId(orderId int64) {}
func (w *simpleWrapper) ManagedAccounts(accountsList string) {}
func (w *simpleWrapper) Error(reqId int, code int, msg string) {
	if code != 2104 && code != 2106 && code != 2168 { // Ignore common warning codes
		fmt.Printf("TWS Warning/Error [%d]: %d %s\n", reqId, code, msg)
	}
}
func (w *simpleWrapper) CurrentTime(timeInSeconds int64) {}
func (w *simpleWrapper) ContractDetails(reqId int64, details tws.ContractDetails) {}
func (w *simpleWrapper) ContractDetailsEnd(reqId int64) {}
func (w *simpleWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {}
func (w *simpleWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {}
func (w *simpleWrapper) AccountSummary(reqId int64, account, tag, value, currency string) {}
func (w *simpleWrapper) AccountSummaryEnd(reqId int64) {}
func (w *simpleWrapper) Position(account string, contract tws.Contract, position decimal.Decimal, avgCost float64) {}
func (w *simpleWrapper) PositionEnd() {}
func (w *simpleWrapper) OpenOrder(orderId int64, contract tws.Contract, order tws.Order, orderState tws.OrderState) {}
func (w *simpleWrapper) OpenOrderEnd() {}
func (w *simpleWrapper) OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64) {}


func main() {
	fmt.Println("=== Phase 3.2 Live Test ===")
	
	client := tws.NewClient("127.0.0.1", 4002, 6)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("Connecting to TWS Paper (port 4002)...")
	client.AddWrapper(&simpleWrapper{})
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Wait for connection to settle
	time.Sleep(2 * time.Second)

	fmt.Println("Connected successfully.")

	tradingSvc := services.NewIBKRTradingService(client)

	fmt.Println("\n--- 1. Testing GetAccount ---")
	accCtx, accCancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer accCancel()
	acc, err := tradingSvc.GetAccount(accCtx)
	if err != nil {
		fmt.Printf("GetAccount Error: %v\n", err)
	} else {
		fmt.Printf("Account: %s\n", acc.ID)
		fmt.Printf("Net Liquidation: $%.2f\n", acc.PortfolioValue)
		fmt.Printf("Cash: $%.2f\n", acc.Cash)
		fmt.Printf("Buying Power: $%.2f\n", acc.BuyingPower)
	}

	fmt.Println("\n--- 2. Testing GetPositions ---")
	posCtx, posCancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer posCancel()
	positions, err := tradingSvc.GetPositions(posCtx)
	if err != nil {
		fmt.Printf("GetPositions Error: %v\n", err)
	} else {
		if len(positions) == 0 {
			fmt.Println("No active positions.")
		} else {
			for _, p := range positions {
				fmt.Printf("- %s: %.2f @ avg cost %.2f\n", p.Symbol, p.Qty, p.AvgEntryPrice)
			}
		}
	}

	fmt.Println("\n--- 3. Testing ListOrders ---")
	ordCtx, ordCancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer ordCancel()
	orders, err := tradingSvc.ListOrders(ordCtx, "")
	if err != nil {
		fmt.Printf("ListOrders Error: %v\n", err)
	} else {
		if len(orders) == 0 {
			fmt.Println("No open orders.")
		} else {
			for _, o := range orders {
				fmt.Printf("- Order %s: %s %.2f %s (%s) - %s\n", o.ID, o.Side, o.Qty, o.Symbol, o.Type, o.Status)
			}
		}
	}

	fmt.Println("\n--- 4. Testing PlaceOrder (Paper Only) ---")
	placeCtx, placeCancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer placeCancel()
	
	order := &interfaces.Order{
		Symbol: "AAPL",
		Qty:    1.0,
		Side:   "buy",
		Type:   "LMT",
	}
	price := 10.0 // Low limit price so it doesn't fill immediately, allowing us to cancel it
	order.LimitPrice = &price

	res, err := tradingSvc.PlaceOrder(placeCtx, order)
	if err != nil {
		fmt.Printf("PlaceOrder Error: %v\n", err)
	} else {
		fmt.Printf("Order placed successfully! Result: %+v\n", res)
		
		// Wait a bit to receive OrderStatus and OpenOrder callbacks
		fmt.Println("Waiting 3 seconds for OpenOrder and OrderStatus callbacks...")
		time.Sleep(3 * time.Second)

		fmt.Println("\n--- 6. Testing CancelOrder ---")
		cancelCtx, cancelCtxCancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancelCtxCancel()
		
		err := tradingSvc.CancelOrder(cancelCtx, res.OrderID)
		if err != nil {
			fmt.Printf("CancelOrder Error: %v\n", err)
		} else {
			fmt.Printf("CancelOrder sent for %s. Waiting 2 seconds for cancellation callbacks...\n", res.OrderID)
			time.Sleep(2 * time.Second)
		}
	}

	fmt.Println("\n=== Test Complete ===")
}
