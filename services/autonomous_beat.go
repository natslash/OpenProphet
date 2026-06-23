package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"prophet-trader/interfaces"

	"github.com/sirupsen/logrus"
)

type AutonomousBeatConfig struct {
	Interval           time.Duration
	MaxDailyExecutions int
	LLMPollingEnabled  bool
	LLMPollingInterval time.Duration
}

type AutonomousBeat struct {
	data    interfaces.DataService
	trading interfaces.TradingService
	pm      *PositionManager
	logger  *logrus.Logger
	cfg     AutonomousBeatConfig

	llm interfaces.LLMProvider
	intentManager        *IntentManager
	requireDoubleConfirm bool

	mu           sync.Mutex
	isRunning    bool
	cancel       context.CancelFunc
	messageQueue []string
	triggerCh    chan struct{}
	lastPollTime time.Time
}

func NewAutonomousBeat(data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService, logger *logrus.Logger, cfg AutonomousBeatConfig, intentManager *IntentManager, requireDoubleConfirm bool) *AutonomousBeat {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.MaxDailyExecutions <= 0 {
		cfg.MaxDailyExecutions = 3 // To prevent AI from going crazy
	}

	var llm interfaces.LLMProvider
	var err error

	providerType := os.Getenv("LLM_PROVIDER")
	if providerType == "gemini" {
		llm, err = NewGeminiClient()
		if err != nil {
			logger.WithError(err).Warn("Failed to initialize Gemini client")
		}
	} else {
		llm, err = NewAnthropicClient()
		if err != nil {
			logger.WithError(err).Warn("Failed to initialize Anthropic client")
		}
	}

	return &AutonomousBeat{
		data:    data,
		trading: trading,
		pm:      pm,
		logger:  logger,
		cfg:     cfg,
		llm:     llm,
		intentManager: intentManager,
		requireDoubleConfirm: requireDoubleConfirm,
	}
}

// Start spawns the heartbeat in the background
func (b *AutonomousBeat) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isRunning {
		return fmt.Errorf("already running")
	}
	if b.llm == nil {
		return fmt.Errorf("LLM provider not initialized")
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.isRunning = true

	go b.Run(ctx)
	return nil
}

func (b *AutonomousBeat) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isRunning {
		return fmt.Errorf("not running")
	}
	b.cancel()
	b.isRunning = false
	return nil
}

func (b *AutonomousBeat) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.isRunning
}

func (b *AutonomousBeat) Interval() time.Duration {
	return b.cfg.Interval
}

func (b *AutonomousBeat) Run(ctx context.Context) {
	b.logger.WithFields(logrus.Fields{
		"interval": b.cfg.Interval,
	}).Warn("[BEAT] Native AI autonomous beat started")

	// Trigger immediate tick on start
	b.tick(ctx)

	t := time.NewTicker(b.cfg.Interval)
	defer t.Stop()

	b.triggerCh = make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			b.logger.Info("AI beat loop canceled")
			return
		case <-t.C:
			b.tick(ctx)
		case <-b.triggerCh:
			b.tick(ctx)
		}
	}
}

func (b *AutonomousBeat) InjectMessage(msg string) {
	b.mu.Lock()
	b.messageQueue = append(b.messageQueue, msg)
	b.mu.Unlock()
	select {
	case b.triggerCh <- struct{}{}:
	default:
	}
}

func (b *AutonomousBeat) tick(ctx context.Context) {
	b.mu.Lock()
	var pending []string
	if len(b.messageQueue) > 0 {
		pending = b.messageQueue
		b.messageQueue = nil
	}
	b.mu.Unlock()

	// Check for automated LLM polling if no pending user messages
	if len(pending) == 0 {
		if !b.cfg.LLMPollingEnabled {
			return
		}

		if time.Since(b.lastPollTime) < b.cfg.LLMPollingInterval {
			return
		}

		// Conservation: the automated review exists to manage OPEN positions.
		// If the book is positively flat, skip the entire ~10-turn LLM loop
		// rather than burn it analysing an empty portfolio. We only skip when
		// both position reads succeed and are empty — on any error we can't be
		// sure, so we run the review (fail safe, not fail cheap).
		if b.portfolioIsFlat(ctx) {
			b.logger.Info("[BEAT] Portfolio flat — skipping automated review to conserve tokens")
			b.lastPollTime = time.Now()
			return
		}

		b.logger.Info("[BEAT] Triggering automated LLM portfolio review")
		pending = append(pending, "Automated system trigger: Please review my current active positions and suggest any strategic adjustments or exit strategies based on current market data. Use the get_positions tool.")
		b.lastPollTime = time.Now()
	}

	b.logger.Info("[BEAT] AI Heartbeat executing for user prompt/automated review...")

	// 1. Build System Prompt from TRADING_RULES.md
	rulesData, err := os.ReadFile("TRADING_RULES.md")
	var systemPrompt string
	if err == nil {
		systemPrompt = string(rulesData)
	} else {
		b.logger.WithError(err).Warn("Could not read TRADING_RULES.md, using default prompt")
		systemPrompt = "You are OpenProphet, an autonomous trading agent."
	}

	systemPrompt += "\n\nCRITICAL CONTEXT:\n- Timezone: CET (Central European Time)\n- Base Currency: EUR (€)\nEnsure all price values, portfolio calculations, and temporal reasoning naturally default to Euros and CET without requiring manual prompting."

	// 2. Build User Text with volatile context
	userText := fmt.Sprintf("Current Time: %s\n\n", time.Now().Format(time.RFC3339))
	userText += "User terminal instructions:\n"
	for _, msg := range pending {
		userText += "- " + msg + "\n"
	}

	messages := []interfaces.LLMMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
	}

	tools := BuildAgentTools()

	// 2. Loop until AI stops calling tools. concluded tracks whether the model
	// finished on its own (a turn with no tool calls). If it instead burns the
	// whole turn budget still calling tools, we force a final summary below so
	// the user never gets silence.
	concluded := false
	jimRogersCalls := 0
	const maxTurns = 10
	for i := 0; i < maxTurns; i++ {
		if ctx.Err() != nil {
			return
		}

		resp, err := b.llm.GenerateResponse(ctx, messages, tools)
		if err != nil {
			b.logger.WithError(err).Error("[BEAT] LLM API error")

			// Emit error to the UI
			appendJSONToBotLog("agent_text", "", fmt.Sprintf("Error interacting with LLM: %v", err))
			return
		}

		// Append LLM's response to the conversation
		messages = append(messages, interfaces.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		if resp.Content != "" {
			b.logger.WithField("ai_thought", resp.Content).Info("[BEAT] AI Log")
			// Emit text to the UI
			appendJSONToBotLog("agent_text", "", resp.Content)
		}

		if len(resp.ToolCalls) == 0 {
			// LLM is done
			b.logger.Info("[BEAT] AI Heartbeat complete")
			concluded = true
			break
		}

		for _, toolCall := range resp.ToolCalls {
			// Cap sub-agent consultations per cycle — each is a full nested LLM
			// call. Beyond the limit, short-circuit with a note instead of
			// executing, so the agent stops fanning out and concludes.
			if toolCall.Name == "jim_rogers" {
				jimRogersCalls++
				if jimRogersCalls > maxJimRogersPerCycle {
					b.logger.Warn("[BEAT] jim_rogers consultation limit reached for this cycle")
					messages = append(messages, interfaces.LLMMessage{
						Role:         "user",
						Content:      "You have reached the limit for consulting other agents (jim_rogers) this cycle. Do not call it again; proceed with your own analysis and give your recommendation.",
						ToolResultID: toolCall.ID,
					})
					continue
				}
			}

			b.logger.WithField("tool", toolCall.Name).Info("[BEAT] AI executing tool")

			// Emit tool use to the UI
			appendToolToBotLog("", toolCall.Name, toolCall.Arguments)

			resStr, toolErr := HandleToolCall(ctx, toolCall.Name, toolCall.Arguments, b.data, b.pm, b.trading, b.llm, b.intentManager, b.requireDoubleConfirm)

			var resultMsg string
			if toolErr != nil {
				resultMsg = fmt.Sprintf("Error: %s", toolErr.Error())
				b.logger.WithError(toolErr).WithField("tool", toolCall.Name).Warn("[BEAT] Tool failed")
			} else {
				resultMsg = resStr
			}

			// In this generic interface, we'll append the tool result as a user
			// message. Truncate oversized results before they enter the
			// transcript: the full history is re-sent on every subsequent turn,
			// so an unbounded tool dump compounds into O(n²) token usage.
			messages = append(messages, interfaces.LLMMessage{
				Role:         "user",
				Content:      truncateForHistory(resultMsg),
				ToolResultID: toolCall.ID,
			})
		}
	}

	// Fallback: the model exhausted its tool-turn budget without delivering a
	// final answer. Force one summarization turn with tools disabled so it must
	// reply in text — otherwise the user is left staring at a silent chat.
	if !concluded {
		b.logger.Warn("[BEAT] Tool-turn limit reached without a final answer — forcing a summary")
		messages = append(messages, interfaces.LLMMessage{
			Role:    "user",
			Content: "You have reached your tool-use limit for this cycle. Do NOT call any more tools. Based only on the data you have already gathered, give the user your final recommendation now.",
		})
		resp, err := b.llm.GenerateResponse(ctx, messages, nil) // nil tools → the model must answer in text
		if err != nil {
			b.logger.WithError(err).Error("[BEAT] Forced summary failed")
			appendJSONToBotLog("agent_text", "", "I gathered the market data but reached my analysis-step limit before finishing. Please narrow the request or ask again.")
			return
		}
		if resp.Content != "" {
			b.logger.WithField("ai_thought", resp.Content).Info("[BEAT] AI Log (forced summary)")
			appendJSONToBotLog("agent_text", "", resp.Content)
		} else {
			appendJSONToBotLog("agent_text", "", "I gathered the market data but reached my analysis-step limit before finishing. Please narrow the request or ask again.")
		}
	}
}

// maxToolResultChars caps how much of a single tool result is carried forward
// in the conversation history (~1.5k tokens). Tool results are re-sent on every
// later turn, so leaving them unbounded compounds token cost across the loop.
const maxToolResultChars = 6000

// maxJimRogersPerCycle caps sub-agent (jim_rogers) consultations per beat. Each
// one is a full nested LLM call whose output is appended to the transcript, so
// uncapped use multiplies token cost.
const maxJimRogersPerCycle = 2

// portfolioIsFlat reports true only when both position reads succeed and are
// empty. On any error it returns false so the caller runs the review rather
// than skipping it on an unconfirmed assumption.
func (b *AutonomousBeat) portfolioIsFlat(ctx context.Context) bool {
	if b.trading == nil {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pos, err := b.trading.GetPositions(cctx)
	if err != nil {
		return false
	}
	opos, err := b.trading.ListOptionsPositions(cctx)
	if err != nil {
		return false
	}
	return len(pos) == 0 && len(opos) == 0
}

func truncateForHistory(s string) string {
	if len(s) <= maxToolResultChars {
		return s
	}
	return s[:maxToolResultChars] + fmt.Sprintf("\n...[truncated %d chars to conserve context]", len(s)-maxToolResultChars)
}

func appendJSONToBotLog(event string, sandboxId string, text string) {
	f, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		data := map[string]interface{}{
			"event": event,
			"data": map[string]interface{}{
				"sandboxId": sandboxId,
				"text":      text,
			},
		}
		b, _ := json.Marshal(data)
		f.WriteString(string(b) + "\n")
	}
}

func appendToolToBotLog(sandboxId string, toolName string, args []byte) {
	f, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		var argsMap map[string]interface{}
		json.Unmarshal(args, &argsMap)
		
		data := map[string]interface{}{
			"event": "agent_tool",
			"data": map[string]interface{}{
				"sandboxId": sandboxId,
				"tool":      toolName,
				"args":      argsMap,
			},
		}
		b, _ := json.Marshal(data)
		f.WriteString(string(b) + "\n")
	}
}
