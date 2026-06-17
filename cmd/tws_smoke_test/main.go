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

func main() {
	fmt.Println("Starting Phase 2.2 Smoke Test...")
	client := tws.NewClient("127.0.0.1", 4002, 2)
	wrapper := &smokeTestWrapper{timeReceived: make(chan int64, 1)}
	client.SetWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("FAIL: Connect error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected. Sending ReqCurrentTime...")
	encoder := client.Encoder()
	if err := encoder.ReqCurrentTime(); err != nil {
		fmt.Printf("FAIL: ReqCurrentTime error: %v\n", err)
		os.Exit(1)
	}

	select {
	case tMs := <-wrapper.timeReceived:
		fmt.Printf("OK: Received CurrentTime: %d\n", tMs)
	case <-time.After(5 * time.Second):
		fmt.Println("FAIL: Timed out waiting for CurrentTime")
		os.Exit(1)
	}
	client.Close()
	fmt.Println("Smoke test completed successfully.")
}
