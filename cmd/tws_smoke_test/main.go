package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/tws"
	"time"
	"github.com/shopspring/decimal"
)

type smokeTestWrapper struct {
	timeReceived chan int64
}

func (s *smokeTestWrapper) CurrentTime(timeInSeconds int64) {
	s.timeReceived <- timeInSeconds
}
func (s *smokeTestWrapper) NextValidId(orderId int64) {}
func (s *smokeTestWrapper) ManagedAccounts(accountsList string) {}
func (s *smokeTestWrapper) Error(reqId int, code int, msg string) {
	fmt.Printf("Error: [%d] %d %s\n", reqId, code, msg)
}
func (s *smokeTestWrapper) ContractDetails(reqId int64, details tws.ContractDetails) {}
func (s *smokeTestWrapper) ContractDetailsEnd(reqId int64) {}

func (s *smokeTestWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {
	fmt.Printf("TickPrice: reqId=%d tickType=%d price=%f size=%s attr=%+v\n", reqId, tickType, price, size.String(), attr)
}

func (w *smokeTestWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	fmt.Printf("SMOKE: Tick Size: req=%d type=%d size=%v\n", reqId, tickType, size)
}
func (w *smokeTestWrapper) AccountSummary(reqId int64, account, tag, value, currency string) {}
func (w *smokeTestWrapper) AccountSummaryEnd(reqId int64) {}
func (w *smokeTestWrapper) Position(account string, contract tws.Contract, position decimal.Decimal, avgCost float64) {}
func (w *smokeTestWrapper) PositionEnd() {}
func (w *smokeTestWrapper) OpenOrder(orderId int64, contract tws.Contract, order tws.Order, orderState tws.OrderState) {}
func (w *smokeTestWrapper) OpenOrderEnd() {}
func (w *smokeTestWrapper) OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64) {}

func main() {
	fmt.Println("Starting Phase 2.5 Smoke Test...")
	client := tws.NewClient("127.0.0.1", 4002, 5)
	wrapper := &smokeTestWrapper{timeReceived: make(chan int64, 1)}
	client.SetWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected. Requesting Market Data for AAPL...")
	contract := tws.Contract{Symbol: "AAPL", SecType: tws.Stock, Exchange: "SMART", Currency: "USD"}
	
	reqId := client.NextOrderId()
	err := client.Encoder().ReqMktData(reqId, contract, "", false, false)
	if err != nil {
		fmt.Printf("FAIL: ReqMktData error: %v\n", err)
		os.Exit(1)
	}

	// Wait to see ticks
	time.Sleep(3 * time.Second)

	fmt.Println("Cancelling Market Data...")
	_ = client.Encoder().CancelMktData(reqId)
	
	client.Close()
	fmt.Println("Smoke test completed successfully.")
}
