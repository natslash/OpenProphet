package services

import (
	"context"
	"testing"
	"strings"

	"prophet-trader/interfaces"
	"github.com/stretchr/testify/assert"
)

// mockTradingService for tool test
type mockTradingService struct {
	interfaces.TradingService
}

func (m *mockTradingService) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	return &interfaces.Account{
		ID:             "DU123",
		Cash:           1000.0,
		PortfolioValue: 2000.0,
		BuyingPower:    4000.0,
	}, nil
}

func TestHandleToolCall_GetAccount(t *testing.T) {
	mockTrading := &mockTradingService{}
	res, err := HandleToolCall(context.Background(), "get_account", []byte(`{}`), nil, nil, mockTrading)
	
	assert.NoError(t, err)
	assert.True(t, strings.Contains(res, "DU123"))
	assert.True(t, strings.Contains(res, "4000"))
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	_, err := HandleToolCall(context.Background(), "some_unknown_tool", []byte(`{}`), nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}
