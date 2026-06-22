package database

import (
	"prophet-trader/models"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *LocalStorage {
	db, err := gorm.Open(sqlite.Open("file::memory:?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.DBPosition{},
		&models.DBTrade{},
		&models.DBManagedPosition{},
	)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	return &LocalStorage{db: db}
}

func Test_SaveAndLoadManagedPosition(t *testing.T) {
	storage := setupTestDB(t)

	original := &models.DBManagedPosition{
		PositionID:        "pos_12345",
		Symbol:            "ESTX50",
		Side:              "buy",
		Strategy:          "SWING_TRADE",
		Quantity:          10,
		EntryPrice:        6325.50,
		EntryOrderID:      "order_1",
		EntryOrderType:    "market",
		AllocationDollars: 63255.00,
		StopLossPrice:     6000.00,
		StopLossPercent:   5.14,
		TrailingStop:      true,
		TakeProfitPrice:   6800.00,
		TakeProfitPercent: 7.50,
		Status:            "ACTIVE",
		CurrentPrice:      6400.00,
		UnrealizedPL:      745.00,
		UnrealizedPLPC:    1.17,
		RemainingQty:      10,
		Notes:             "Test Position",
	}

	err := storage.SaveManagedPosition(original)
	require.NoError(t, err)

	loaded, err := storage.GetManagedPosition("pos_12345")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, original.PositionID, loaded.PositionID)
	assert.Equal(t, original.Symbol, loaded.Symbol)
	assert.Equal(t, original.Side, loaded.Side)
	assert.Equal(t, original.Quantity, loaded.Quantity)
	assert.Equal(t, original.TrailingStop, loaded.TrailingStop)
	assert.Equal(t, original.TakeProfitPrice, loaded.TakeProfitPrice)
	assert.Equal(t, original.Status, loaded.Status)
}

func Test_StatusFiltering(t *testing.T) {
	storage := setupTestDB(t)

	positions := []*models.DBManagedPosition{
		{PositionID: "pos_1", Status: "ACTIVE"},
		{PositionID: "pos_2", Status: "ACTIVE"},
		{PositionID: "pos_3", Status: "PENDING"},
		{PositionID: "pos_4", Status: "CLOSED"},
		{PositionID: "pos_5", Status: "CLOSED"},
	}

	for _, p := range positions {
		err := storage.SaveManagedPosition(p)
		require.NoError(t, err)
	}

	activePos, err := storage.GetAllManagedPositions("ACTIVE")
	require.NoError(t, err)
	assert.Len(t, activePos, 2)

	pendingPos, err := storage.GetAllManagedPositions("PENDING")
	require.NoError(t, err)
	assert.Len(t, pendingPos, 1)
	assert.Equal(t, "pos_3", pendingPos[0].PositionID)

	allPos, err := storage.GetAllManagedPositions("")
	require.NoError(t, err)
	assert.Len(t, allPos, 5)
}

func Test_DBLocking(t *testing.T) {
	storage := setupTestDB(t)

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Simulate 100 concurrent updates to different positions
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pos := &models.DBManagedPosition{
				PositionID:   "pos_concurrent_" + string(rune(idx)),
				Status:       "ACTIVE",
				CurrentPrice: float64(idx),
			}
			if err := storage.SaveManagedPosition(pos); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent save failed: %v", err)
	}

	allPos, err := storage.GetAllManagedPositions("")
	require.NoError(t, err)
	assert.Len(t, allPos, 100)
}
