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
	ctx := context.Background()
	mockTrading := &mockTradingService{}
	res, err := HandleToolCall(ctx, "get_account", []byte(`{}`), nil, nil, mockTrading, nil, nil, false)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(res, "DU123"))
	assert.True(t, strings.Contains(res, "4000"))
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	ctx := context.Background()
	_, err := HandleToolCall(ctx, "unknown_tool", []byte(`{}`), nil, nil, nil, nil, nil, false)
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

// cappedMockLLM never voluntarily concludes: as long as tools are offered it
// keeps requesting one, exhausting the turn budget. When called with no tools
// (the forced summary turn) it returns text.
type cappedMockLLM struct {
	calls              int
	toolTurns          int
	forcedSummaryCalls int
}

func (m *cappedMockLLM) GetName() string { return "capped-mock" }
func (m *cappedMockLLM) GenerateResponse(ctx context.Context, messages []interfaces.LLMMessage, tools []interfaces.LLMTool) (*interfaces.LLMResponse, error) {
	m.calls++
	if len(tools) == 0 {
		// Forced-summary turn: must produce text.
		m.forcedSummaryCalls++
		return &interfaces.LLMResponse{Content: "FORCED FINAL SUMMARY"}, nil
	}
	// Normal turn: keep calling a tool, never finish on our own.
	m.toolTurns++
	return &interfaces.LLMResponse{
		ToolCalls: []interfaces.LLMToolCall{{ID: "t1", Name: "get_account", Arguments: []byte(`{}`)}},
	}, nil
}

// When the model burns its whole tool budget without concluding, the beat must
// force exactly one tools-disabled summary turn so the user still gets a reply.
func TestAutonomousBeat_ForcesSummaryOnTurnLimit(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	mockLLM := &cappedMockLLM{}

	beat := &AutonomousBeat{
		llm:     mockLLM,
		trading: &mockTradingService{},
		logger:  logger,
		cfg:     AutonomousBeatConfig{},
	}
	beat.InjectMessage("recommend a trade")

	beat.tick(context.Background())

	// 10 tool turns (the cap) + exactly one forced summary turn.
	assert.Equal(t, 10, mockLLM.toolTurns, "should exhaust the 10-turn tool budget")
	assert.Equal(t, 1, mockLLM.forcedSummaryCalls, "should force exactly one summary turn")
	assert.Equal(t, 11, mockLLM.calls)
}

// When the model concludes on its own (a turn with no tool calls), the beat must
// NOT trigger the forced summary.
func TestAutonomousBeat_NoForcedSummaryWhenConcluded(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	mockLLM := &mockLLMProvider{} // returns text, no tool calls → concludes immediately

	beat := &AutonomousBeat{
		llm:     mockLLM,
		trading: &mockTradingService{},
		logger:  logger,
		cfg:     AutonomousBeatConfig{},
	}
	beat.InjectMessage("recommend a trade")

	beat.tick(context.Background())

	assert.True(t, mockLLM.called)
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
