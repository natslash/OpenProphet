package tws

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// simpleTestWrapper is a basic wrapper to catch errors during integration tests
type simpleTestWrapper struct {
	DefaultWrapper
	errors []string
}

func (w *simpleTestWrapper) Error(reqId int, code int, msg string) {
	switch code {
	case 2104, 2106, 2158, 2107, 2119: // Routine connection/market data notices
		return
	default:
		w.errors = append(w.errors, fmt.Sprintf("Error %d: %s", code, msg))
	}
}

// TestIntegration_ConnectAndDisconnect tests that the client can physically connect to TWS on port 4002.
// Requires TWS or IB Gateway to be running locally on port 4002.
func TestIntegration_ConnectAndDisconnect(t *testing.T) {
	client := NewClient("127.0.0.1", 4002, 11)
	wrapper := &simpleTestWrapper{}
	client.AddWrapper(wrapper)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to TWS (ensure TWS is running on port 4002): %v", err)
	}

	time.Sleep(1 * time.Second)

	if !client.connected {
		t.Errorf("Expected client.connected to be true after successful connect")
	}

	client.Close()
	
	if client.connected {
		t.Errorf("Expected client.connected to be false after Close()")
	}
}
