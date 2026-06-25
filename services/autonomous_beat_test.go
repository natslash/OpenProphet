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

func (m *mockTradingService) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	return nil, nil
}

func TestHandleToolCall_GetAccount(t *testing.T) {
	ctx := context.Background()
	tc := &ToolContext{Trading: &mockTradingService{}}
	res, err := HandleToolCall(ctx, "get_account", []byte(`{}`), tc)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(res, "DU123"))
	assert.True(t, strings.Contains(res, "4000"))
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	ctx := context.Background()
	tc := &ToolContext{}
	_, err := HandleToolCall(ctx, "unknown_tool", []byte(`{}`), tc)
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

// jimSpamLLM always asks for jim_rogers (while tools are offered) and records
// whether it was ever told it hit the consultation cap.
type jimSpamLLM struct{ sawCapMessage bool }

func (m *jimSpamLLM) GetName() string { return "jim-spam" }
func (m *jimSpamLLM) GenerateResponse(ctx context.Context, messages []interfaces.LLMMessage, tools []interfaces.LLMTool) (*interfaces.LLMResponse, error) {
	for _, msg := range messages {
		if strings.Contains(msg.Content, "limit for consulting other agents") {
			m.sawCapMessage = true
		}
	}
	if len(tools) == 0 {
		return &interfaces.LLMResponse{Content: "final"}, nil // forced summary turn
	}
	return &interfaces.LLMResponse{
		ToolCalls: []interfaces.LLMToolCall{{ID: "j", Name: "jim_rogers", Arguments: []byte(`{"target_agent_id":"x","prompt":"y"}`)}},
	}, nil
}

func TestAutonomousBeat_CapsJimRogers(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	mock := &jimSpamLLM{}
	beat := &AutonomousBeat{llm: mock, trading: &mockTradingService{}, logger: logger, cfg: AutonomousBeatConfig{}}
	beat.InjectMessage("recommend")

	beat.tick(context.Background())

	assert.True(t, mock.sawCapMessage, "agent should be told it hit the jim_rogers cap after %d consultations", maxJimRogersPerCycle)
}

// flatTradingMock confirms an empty book; heldTradingMock reports one position.
type flatTradingMock struct{ interfaces.TradingService }

func (flatTradingMock) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	return nil, nil
}
func (flatTradingMock) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, nil
}

type heldTradingMock struct{ interfaces.TradingService }

func (heldTradingMock) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	return []*interfaces.Position{{Symbol: "ESTX50"}}, nil
}
func (heldTradingMock) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, nil
}
func (heldTradingMock) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	return &interfaces.Account{ID: "DU123", PortfolioValue: 2000}, nil
}

func TestAutonomousBeat_SkipsReviewWhenFlat(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	mock := &mockLLMProvider{}
	beat := &AutonomousBeat{
		llm: mock, trading: flatTradingMock{}, logger: logger,
		cfg: AutonomousBeatConfig{LLMPollingEnabled: true, LLMPollingInterval: 0},
	}

	beat.tick(context.Background()) // no pending msg → polling path; flat → should skip

	assert.False(t, mock.called, "automated review must be skipped when the book is flat")
}

func TestAutonomousBeat_RunsReviewWhenHoldingPositions(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	mock := &mockLLMProvider{}
	beat := &AutonomousBeat{
		llm: mock, trading: heldTradingMock{}, logger: logger,
		cfg: AutonomousBeatConfig{LLMPollingEnabled: true, LLMPollingInterval: 0},
	}

	beat.tick(context.Background())

	assert.True(t, mock.called, "review should run when positions are open")
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
