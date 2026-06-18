package main

import (
	"log"
	"os"
	"prophet-trader/tws"
	"time"
	"github.com/shopspring/decimal"
)

type dummyWrapper struct {}
func (d *dummyWrapper) NextValidId(orderId int64) {}
func (d *dummyWrapper) ManagedAccounts(accountsList string) {}
func (d *dummyWrapper) Error(reqId int, code int, msg string) { log.Printf("Err: %d %d %s", reqId, code, msg)}
func (d *dummyWrapper) CurrentTime(timeInSeconds int64) {}
func (d *dummyWrapper) ContractDetails(reqId int64, details tws.ContractDetails) {}
func (d *dummyWrapper) ContractDetailsEnd(reqId int64) {}
func (d *dummyWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {}
func (d *dummyWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {}

func main() {
	log.SetOutput(os.Stdout)
	client := tws.NewClient("127.0.0.1", 4002, 33)
	err := client.Connect()
	if err != nil {
		log.Fatalf("connect error: %v", err)
	}

	contract := tws.Contract{
		Symbol: "ESTX50",
		SecType: "OPT",
		Exchange: "EUREX",
		Currency: "EUR",
		LastTradeDateOrContractMonth: "20260619",
		Strike: 5200,
		Right: "C",
		Multiplier: 10,
		TradingClass: "OESX",
	}

	time.Sleep(1 * time.Second)
	log.Printf("Requesting mkt data")
	err = client.Encoder().ReqMktData(1001, contract, "", false, false)
	if err != nil {
		log.Fatalf("reqmktdata error: %v", err)
	}

	time.Sleep(5 * time.Second)
}
