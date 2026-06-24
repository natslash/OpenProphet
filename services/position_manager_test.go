package services

import (
	"context"
	"prophet-trader/database"
	"prophet-trader/interfaces"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockTradingService struct {
	mock.Mock
}

func (m *MockTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	args := m.Called(ctx, order)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.OrderResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTradingService) CancelOrder(ctx context.Context, orderID string) error {
	args := m.Called(ctx, orderID)
	return args.Error(0)
}

func (m *MockTradingService) GetOrder(ctx context.Context, orderID string) (*interfaces.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.Order), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTradingService) ListOrders(ctx context.Context, status string) ([]*interfaces.Order, error) {
	args := m.Called(ctx, status)
	if args.Get(0) != nil {
		return args.Get(0).([]*interfaces.Order), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTradingService) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).([]*interfaces.Position), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTradingService) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.Account), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockTradingService) PlaceOptionsOrder(ctx context.Context, order *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	return nil, nil
}
func (m *MockTradingService) PlaceComboOrder(ctx context.Context, order *interfaces.ComboOrder) (*interfaces.OrderResult, error) {
	return nil, nil
}
func (m *MockTradingService) GetOptionsChain(ctx context.Context, underlying string, expiration time.Time) ([]*interfaces.OptionContract, error) {
	return nil, nil
}
func (m *MockTradingService) GetOptionsQuote(ctx context.Context, symbol string) (*interfaces.OptionsQuote, error) {
	return nil, nil
}
func (m *MockTradingService) GetOptionsPosition(ctx context.Context, symbol string) (*interfaces.OptionsPosition, error) {
	return nil, nil
}
func (m *MockTradingService) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, nil
}

// SetupTestDB initializes an in-memory SQLite database for testing LocalStorage
func SetupTestDB(t *testing.T) *database.LocalStorage {
	storage, err := database.NewLocalStorage("file::memory:?_busy_timeout=5000&_journal_mode=WAL")
	require.NoError(t, err)
	return storage
}

// Test_RuleEngine_HitTakeProfit proves that the Go backend natively triggers a close if profit is hit
func Test_RuleEngine_HitTakeProfit(t *testing.T) {
	mockTrading := new(MockTradingService)
	mockData := new(MockDataService)
	
	// Create an actual in-memory SQLite DB for the test
	mockStorage := SetupTestDB(t)

	pm := &PositionManager{
		tradingService: mockTrading,
		dataService:    mockData,
		storageService: mockStorage,
		positions:      make(map[string]*ManagedPosition),
		logger:         logrus.New(),
	}

	// Setup an ACTIVE position that was bought at 100, TakeProfit is 150.
	posID := "pos_take_profit"
	pm.positions[posID] = &ManagedPosition{
		ID:              posID,
		Symbol:          "ESTX50",
		Side:            "buy",
		Status:          "ACTIVE",
		Quantity:        10,
		EntryPrice:      100.0,
		TakeProfitPrice: 150.0,
		StopLossPrice:   50.0,
		RemainingQty:    10,
	}

	// Mock the price rocketing to 160 (Hit Take Profit)
	mockData.On("GetLatestQuote", mock.Anything, "ESTX50").Return(&interfaces.Quote{AskPrice: 160.0}, nil)
	
	// Verify that the rule engine catches the 160 price and fires a SELL market order to close
	mockTrading.On("PlaceOrder", mock.Anything, mock.MatchedBy(func(order *interfaces.Order) bool {
		return order.Symbol == "ESTX50" && order.Side == "sell" && order.Type == "market" && order.Qty == 10
	})).Return(&interfaces.OrderResult{OrderID: "exit_1"}, nil).Once()

	// Run checkPositions manually
	pm.checkPositions(context.Background())

	mockTrading.AssertExpectations(t)
	mockData.AssertExpectations(t)

	// The position should be marked as closed
	assert.Equal(t, "CLOSED", pm.positions[posID].Status)
}

// Test_RuleEngine_HitStopLoss proves that the Go backend natively triggers a stop loss
func Test_RuleEngine_HitStopLoss(t *testing.T) {
	mockTrading := new(MockTradingService)
	mockData := new(MockDataService)
	mockStorage := SetupTestDB(t)

	pm := &PositionManager{
		tradingService: mockTrading,
		dataService:    mockData,
		storageService: mockStorage,
		positions:      make(map[string]*ManagedPosition),
		logger:         logrus.New(),
	}

	// Setup an ACTIVE position that was bought at 100, StopLoss is 50.
	posID := "pos_stop_loss"
	pm.positions[posID] = &ManagedPosition{
		ID:              posID,
		Symbol:          "AAPL",
		Side:            "buy",
		Status:          "ACTIVE",
		Quantity:        5,
		EntryPrice:      100.0,
		TakeProfitPrice: 150.0,
		StopLossPrice:   50.0,
		RemainingQty:    5,
	}

	// Mock the price crashing to 40 (Hit Stop Loss)
	mockData.On("GetLatestQuote", mock.Anything, "AAPL").Return(&interfaces.Quote{AskPrice: 40.0}, nil)
	
	// Verify that the rule engine catches the 40 price and fires a SELL market order to stop loss
	mockTrading.On("PlaceOrder", mock.Anything, mock.MatchedBy(func(order *interfaces.Order) bool {
		return order.Symbol == "AAPL" && order.Side == "sell" && order.Type == "market" && order.Qty == 5
	})).Return(&interfaces.OrderResult{OrderID: "exit_sl_1"}, nil).Once()

	// Run checkPositions
	pm.checkPositions(context.Background())

	mockTrading.AssertExpectations(t)

	// The position should be marked as CLOSED
	assert.Equal(t, "CLOSED", pm.positions[posID].Status)
}
