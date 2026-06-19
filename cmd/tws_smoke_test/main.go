package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/services"
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

func (s *smokeTestWrapper) HistoricalData(reqId int64, bar tws.HistoricalBar) {
	fmt.Printf("HistoricalData: reqId=%d date=%s open=%f close=%f\n", reqId, bar.Date, bar.Open, bar.Close)
}

func (s *smokeTestWrapper) HistoricalDataEnd(reqId int64, startDateStr, endDateStr string) {
	fmt.Printf("HistoricalDataEnd: reqId=%d start=%s end=%s\n", reqId, startDateStr, endDateStr)
}

func (s *smokeTestWrapper) HistoricalDataUpdate(reqId int64, bar tws.HistoricalBar) {
	fmt.Printf("HistoricalDataUpdate: reqId=%d date=%s open=%f close=%f\n", reqId, bar.Date, bar.Open, bar.Close)
}

func main() {
	fmt.Println("Starting Phase 2.5 Smoke Test...")
	client := tws.NewClient("127.0.0.1", 4002, 5)
	wrapper := &smokeTestWrapper{timeReceived: make(chan int64, 1)}
	client.AddWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected. Initializing Data Service...")
	dataService := services.NewIBKRDataService(client)

	fmt.Println("Requesting Historical Data for AAPL via DataService...")
	
	// Query the last 5 days of 1-day bars
	start := time.Now().Add(-5 * 24 * time.Hour)
	end := time.Now()
	
	bars, err := dataService.GetHistoricalBars(ctx, "AAPL", start, end, "1Day")
	if err != nil {
		fmt.Printf("FAIL: GetHistoricalBars error: %v\n", err)
		os.Exit(1)
	}
	for _, b := range bars {
		fmt.Printf("Bar: Time=%v Open=%.2f Close=%.2f Vol=%d\n", b.Timestamp.Local(), b.Open, b.Close, b.Volume)
	}

	fmt.Println("Requesting Latest Bar for AAPL via DataService...")
	latest, err := dataService.GetLatestBar(ctx, "AAPL")
	if err != nil {
		fmt.Printf("FAIL: GetLatestBar error: %v\n", err)
	} else {
		fmt.Printf("Latest Bar: Time=%v Open=%.2f Close=%.2f\n", latest.Timestamp.Local(), latest.Open, latest.Close)
	}

	// Query a small intraday window for OESX
	oesxSymbol := "ESTX50:20260619:C:5200"
	oesxStart := time.Now().Add(-1 * time.Hour)
	oesxBars, err := dataService.GetHistoricalBars(ctx, oesxSymbol, oesxStart, end, "1Min")
	if err != nil {
		fmt.Printf("FAIL: GetHistoricalBars OESX error: %v\n", err)
	} else {
		for _, b := range oesxBars {
			fmt.Printf("OESX Bar: Time=%v Open=%.2f High=%.2f Low=%.2f Close=%.2f Vol=%d\n", 
				b.Timestamp.Local(), b.Open, b.High, b.Low, b.Close, b.Volume)
		}
	}

	client.Close()
	fmt.Println("Smoke test completed successfully.")
}
