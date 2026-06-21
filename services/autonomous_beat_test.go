package services

import (
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"prophet-trader/interfaces"
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
	res, err := HandleToolCall(context.Background(), "get_account", []byte(`{}`), nil, nil, mockTrading, nil)
	
	assert.NoError(t, err)
	assert.True(t, strings.Contains(res, "DU123"))
	assert.True(t, strings.Contains(res, "4000"))
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	_, err := HandleToolCall(context.Background(), "some_unknown_tool", []byte(`{}`), nil, nil, nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

// mockLLMProvider
type mockLLMProvider struct {
	called bool
}

func (m *mockLLMProvider) GetName() string { return "mock" }
func (m *mockLLMProvider) GenerateResponse(ctx context.Context, messages []interfaces.LLMMessage, tools []interfaces.LLMTool) (*interfaces.LLMResponse, error) {
	m.called = true
	return &interfaces.LLMResponse{
		Content: "test response",
	}, nil
}

func TestAutonomousBeat_LLMPolling(t *testing.T) {
	mockLLM := &mockLLMProvider{}
	
	beat := &AutonomousBeat{
		llm: mockLLM,
		logger: logrus.New(),
		cfg: AutonomousBeatConfig{
			LLMPollingEnabled: true,
			LLMPollingInterval: 0, // Fire immediately
		},
	}
	
	// tick should trigger the LLM since pending is empty but polling is enabled
	beat.tick(context.Background())
	
	assert.True(t, mockLLM.called)
}
