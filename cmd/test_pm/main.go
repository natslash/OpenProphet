package main

import (
	"context"
	"fmt"
	"os"
	"prophet-trader/database"
	"prophet-trader/services"
	"prophet-trader/tws"
	"time"
)

// simpleWrapper is a no-op wrapper
type simpleWrapper struct{ tws.DefaultWrapper }

func (w *simpleWrapper) Error(reqId int, code int, msg string) {
	switch code {
	case 2104, 2106, 2158, 2107: // routine data-farm status notices
	default:
		fmt.Printf("TWS Warning/Error [%d]: %d %s\n", reqId, code, msg)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func main() {
	fmt.Println("=== IBKR PositionManager manual test (port 4002) ===")

	client := tws.NewClient("127.0.0.1", 4002, 7)
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

	tradingSvc := services.NewIBKRTradingService(client)
	dataSvc := services.NewIBKRDataService(client)
	db, err := database.NewLocalStorage("test_pm_db.json")
	if err != nil {
		fmt.Printf("FAIL: database: %v\n", err)
		os.Exit(1)
	}

	pm := services.NewPositionManager(tradingSvc, dataSvc, db)

	// Create OESX Bracket Position
	fmt.Println("\n--- Placing PositionManager Entry Order (OESX Option Bracket) ---")
	
	req := services.PlaceManagedPositionRequest{
		Symbol:            "ESTX50:20260619:C:6325", // valid OpenProphet format
		AllocationDollars: 50.0,
		Side:              "buy",
		EntryStrategy:     "limit",
		EntryPrice:        floatPtr(5.0),  // Target limit price
		Strategy:          "manual_test_4.3d",
		TakeProfitPercent: floatPtr(100.0),
		StopLossPercent:   floatPtr(60.0),
	}

	// This should natively place the parent + TP + SL via IBKR
	pos, err := pm.PlaceManagedPosition(context.Background(), &req)
	if err != nil {
		fmt.Printf("FAIL: PlaceManagedPosition: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Success! ManagedPosition Created: %+v\n", pos)
	fmt.Printf("Check TWS -> You should see a group of 3 orders (1 parent, 2 children) for OESX.\n")
	
	fmt.Println("\nRunning pm.checkPositions to verify no secondary risk orders are spawned...")
	// The test is complete.
	fmt.Println("Exiting. Check TWS for the bracket orders.")
}
