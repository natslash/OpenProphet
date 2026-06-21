package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	mu           sync.Mutex
	isRunning    bool
	cancel       context.CancelFunc
	messageQueue []string
	triggerCh    chan struct{}
	lastPollTime time.Time
}

func NewAutonomousBeat(data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService, logger *logrus.Logger, cfg AutonomousBeatConfig) *AutonomousBeat {
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

		b.logger.Info("[BEAT] Triggering automated LLM portfolio review")
		pending = append(pending, "Automated system trigger: Please review my current active positions and suggest any strategic adjustments or exit strategies based on current market data. Use the get_positions tool.")
		b.lastPollTime = time.Now()
	}

	b.logger.Info("[BEAT] AI Heartbeat executing for user prompt/automated review...")

	// 1. Build System Prompt from TRADING_RULES.md
	rulesData, err := ioutil.ReadFile("TRADING_RULES.md")
	var systemPrompt string
	if err == nil {
		systemPrompt = string(rulesData)
	} else {
		b.logger.WithError(err).Warn("Could not read TRADING_RULES.md, using default prompt")
		systemPrompt = "You are OpenProphet, an autonomous trading agent."
	}

	// Add dynamic context
	systemPrompt += fmt.Sprintf("\nCurrent Time: %s", time.Now().Format(time.RFC3339))

	userText := "User terminal instructions:\n"
	for _, msg := range pending {
		userText += "- " + msg + "\n"
	}

	messages := []interfaces.LLMMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
	}

	tools := BuildAgentTools()

	// 2. Loop until AI stops calling tools
	for i := 0; i < 10; i++ { // Max 10 turns to prevent infinite loops
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
			break
		}

		for _, toolCall := range resp.ToolCalls {
			b.logger.WithField("tool", toolCall.Name).Info("[BEAT] AI executing tool")

			// Emit tool use to the UI
			appendToolToBotLog("", toolCall.Name, toolCall.Arguments)

			resStr, toolErr := HandleToolCall(ctx, toolCall.Name, toolCall.Arguments, b.data, b.pm, b.trading, b.llm)

			var resultMsg string
			if toolErr != nil {
				resultMsg = fmt.Sprintf("Error: %s", toolErr.Error())
				b.logger.WithError(toolErr).WithField("tool", toolCall.Name).Warn("[BEAT] Tool failed")
			} else {
				resultMsg = resStr
			}

			// In this generic interface, we'll append the tool result as a user message
			messages = append(messages, interfaces.LLMMessage{
				Role:         "user",
				Content:      resultMsg,
				ToolResultID: toolCall.ID,
			})
		}
	}
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
