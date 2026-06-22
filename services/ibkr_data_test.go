package services

import (
	"context"
	"errors"
	"prophet-trader/interfaces"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCalculateDuration(t *testing.T) {
	base := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		dur     time.Duration
		barSize string
		want    string
	}{
		{"daily 5d", 5 * 24 * time.Hour, "1 day", "5 D"},
		{"1min sub-day 1h", time.Hour, "1 min", "3600 S"},
		{"5min sub-day 30m", 30 * time.Minute, "5 mins", "1800 S"},
		{"1min tiny window floored to 60s", 10 * time.Second, "1 min", "60 S"},
		{"1min multi-day clamped to 2D", 10 * 24 * time.Hour, "1 min", "2 D"},
		{"5min multi-day clamped to 5D", 10 * 24 * time.Hour, "5 mins", "5 D"},
		{"hour sub-day uses seconds", 3 * time.Hour, "1 hour", "10800 S"},
		{"daily sub-day rounds to 1D", time.Hour, "1 day", "1 D"},
		{"daily long maps to years", 800 * 24 * time.Hour, "1 day", "3 Y"},
		{"zero window defensive", 0, "1 min", "1 D"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateDuration(base.Add(-tc.dur), base, tc.barSize)
			if got != tc.want {
				t.Errorf("calculateDuration(dur=%v, %q) = %q, want %q", tc.dur, tc.barSize, got, tc.want)
			}
		})
	}
}

// MockDataService mocks the IBKR Data API
type MockDataService struct {
	mock.Mock
}

func (m *MockDataService) GetLatestQuote(ctx context.Context, symbol string) (*interfaces.Quote, error) {
	args := m.Called(ctx, symbol)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.Quote), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockDataService) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	args := m.Called(ctx, symbol)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.Bar), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockDataService) GetLatestTrade(ctx context.Context, symbol string) (*interfaces.Trade, error) {
	args := m.Called(ctx, symbol)
	if args.Get(0) != nil {
		return args.Get(0).(*interfaces.Trade), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockDataService) StreamBars(ctx context.Context, symbols []string) (<-chan *interfaces.Bar, error) {
	args := m.Called(ctx, symbols)
	if args.Get(0) != nil {
		return args.Get(0).(<-chan *interfaces.Bar), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockDataService) GetHistoricalBars(ctx context.Context, symbol string, start time.Time, end time.Time, barSize string) ([]*interfaces.Bar, error) {
	args := m.Called(ctx, symbol, start, end, barSize)
	if args.Get(0) != nil {
		return args.Get(0).([]*interfaces.Bar), args.Error(1)
	}
	return nil, args.Error(1)
}

// Test_TargetedPolling_Empty ensures 0 network calls when there are no active positions
func Test_TargetedPolling_Empty(t *testing.T) {
	mockData := new(MockDataService)
	
	pm := &PositionManager{
		positions:   make(map[string]*ManagedPosition),
		dataService: mockData,
		logger:      logrus.New(),
	}

	pm.pollActiveQuotes(context.Background())
	mockData.AssertNotCalled(t, "GetLatestQuote")
}

// Test_TargetedPolling_Active ensures it only polls symbols of active positions
func Test_TargetedPolling_Active(t *testing.T) {
	mockData := new(MockDataService)
	pm := &PositionManager{
		positions:   make(map[string]*ManagedPosition),
		dataService: mockData,
		logger:      logrus.New(),
	}

	// Setup active positions
	pm.positions["pos_1"] = &ManagedPosition{Symbol: "ESTX50", Status: "ACTIVE"}
	pm.positions["pos_2"] = &ManagedPosition{Symbol: "AAPL", Status: "PARTIAL"}
	pm.positions["pos_3"] = &ManagedPosition{Symbol: "TSLA", Status: "CLOSED"}

	mockData.On("GetLatestQuote", mock.Anything, "ESTX50").Return(&interfaces.Quote{AskPrice: 6300}, nil).Once()
	mockData.On("GetLatestQuote", mock.Anything, "AAPL").Return(&interfaces.Quote{AskPrice: 150}, nil).Once()

	pm.pollActiveQuotes(context.Background())

	mockData.AssertExpectations(t)
	assert.Equal(t, float64(6300), pm.positions["pos_1"].CurrentPrice)
	assert.Equal(t, float64(150), pm.positions["pos_2"].CurrentPrice)
}

// Test_TargetedPolling_RateLimitFallback ensures graceful failure on HTTP 429
func Test_TargetedPolling_RateLimitFallback(t *testing.T) {
	mockData := new(MockDataService)
	pm := &PositionManager{
		positions:   make(map[string]*ManagedPosition),
		dataService: mockData,
		logger:      logrus.New(),
	}

	pm.positions["pos_1"] = &ManagedPosition{Symbol: "ESTX50", Status: "ACTIVE"}

	// Mock rate limit error
	rateLimitErr := errors.New("HTTP 429: Too Many Requests")
	mockData.On("GetLatestQuote", mock.Anything, "ESTX50").Return((*interfaces.Quote)(nil), rateLimitErr).Once()
	
	// getCurrentPrice falls back to GetLatestBar, so we must mock that too and return an error
	mockData.On("GetLatestBar", mock.Anything, "ESTX50").Return((*interfaces.Bar)(nil), errors.New("no bar")).Once()

	// It should gracefully handle the error and NOT panic
	err := pm.pollActiveQuotes(context.Background())
	
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 429")
	mockData.AssertExpectations(t)
}
