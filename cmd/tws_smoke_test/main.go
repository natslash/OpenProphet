package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/tws"
	"time"
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

func main() {
	fmt.Println("Starting Phase 2.4 Smoke Test...")
	client := tws.NewClient("127.0.0.1", 4002, 5)
	wrapper := &smokeTestWrapper{timeReceived: make(chan int64, 1)}
	client.SetWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected. Sending ReqContractDetails...")
	contract := tws.Contract{Symbol: "ESTX50", SecType: tws.Option, Exchange: "EUREX", Currency: "EUR"}
	
	details, err := client.ReqContractDetails(ctx, contract)
	if err != nil {
		fmt.Printf("FAIL: ReqContractDetails error: %v\n", err)
		os.Exit(1)
	}

	if len(details) > 0 {
		fmt.Printf("OK: Received %d ContractDetails. First ConId: %d, Multiplier: %s\n", len(details), details[0].Contract.ConId, details[0].Contract.Multiplier)
	} else {
		fmt.Println("FAIL: Received 0 ContractDetails.")
		os.Exit(1)
	}
	client.Close()
	fmt.Println("Smoke test completed successfully.")
}
