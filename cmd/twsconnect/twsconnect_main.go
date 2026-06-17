// Command twsconnect is the Phase 2.1 connectivity test. It opens a full API
// session against IB Gateway / TWS (handshake + startApi) and prints the
// negotiated server version, the managed accounts, and the first valid order
// id. It places no orders and requests no market data.
//
// Usage:
//
//	go run ./cmd/twsconnect                 # paper Gateway, 127.0.0.1:4002
//	go run ./cmd/twsconnect -port 7497      # paper TWS desktop
//	go run ./cmd/twsconnect -client 3       # use a different API client id
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"prophet-trader/tws"
)

func main() {
	host := flag.String("host", "127.0.0.1", "IB Gateway / TWS host")
	port := flag.Int("port", 4002, "API port (4002 paper Gateway, 4001 live, 7497 paper TWS)")
	clientID := flag.Int("client", 1, "API client id (must be unique per connection)")
	timeout := flag.Duration("timeout", 10*time.Second, "connect timeout")
	flag.Parse()

	c := tws.NewClient(*host, *port, *clientID)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	fmt.Printf("-> connecting to %s:%d (clientId %d) ...\n", *host, *port, *clientID)
	if err := c.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL  %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	oid := c.NextOrderId()
	fmt.Println("OK  API session started")
	fmt.Printf("    server version : %d\n", c.ServerVersion())
	fmt.Printf("    connection time: %s\n", c.ConnectionTime())
	fmt.Printf("    accounts       : %s\n", c.Accounts())
	fmt.Printf("    next valid id  : %d\n", oid)
	fmt.Println()
	fmt.Println("startApi succeeded and nextValidId received. No orders placed, no data requested.")
}
