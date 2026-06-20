package services

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"prophet-trader/interfaces"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

type AutonomousBeatConfig struct {
	Interval           time.Duration
	MaxDailyExecutions int
}

type AutonomousBeat struct {
	data    interfaces.DataService
	trading interfaces.TradingService
	pm      *PositionManager
	logger  *logrus.Logger
	cfg     AutonomousBeatConfig

	client *AnthropicClient

	mu        sync.Mutex
	isRunning bool
	cancel    context.CancelFunc
}

func NewAutonomousBeat(data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService, logger *logrus.Logger, cfg AutonomousBeatConfig) *AutonomousBeat {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.MaxDailyExecutions <= 0 {
		cfg.MaxDailyExecutions = 3 // To prevent AI from going crazy
	}

	client, err := NewAnthropicClient()
	if err != nil {
		logger.WithError(err).Warn("Failed to initialize Anthropic client, AI beat will be disabled")
	}

	return &AutonomousBeat{
		data:    data,
		trading: trading,
		pm:      pm,
		logger:  logger,
		cfg:     cfg,
		client:  client,
	}
}

// Start spawns the heartbeat in the background
func (b *AutonomousBeat) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isRunning {
		return fmt.Errorf("already running")
	}
	if b.client == nil {
		return fmt.Errorf("AI client not initialized")
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

	for {
		select {
		case <-ctx.Done():
			b.logger.Warn("[BEAT] Native AI autonomous beat stopped")
			return
		case <-t.C:
			b.tick(ctx)
		}
	}
}

func (b *AutonomousBeat) tick(ctx context.Context) {
	b.logger.Info("[BEAT] AI Heartbeat executing...")

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

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("It is time for your heartbeat analysis. Check your account, positions, and market data, and make a decision.")),
	}

	tools := BuildAgentTools()

	// 2. Loop until Claude stops calling tools
	for i := 0; i < 10; i++ { // Max 10 turns to prevent infinite loops
		if ctx.Err() != nil {
			return
		}

		resp, err := b.client.ExecuteAgentTurn(ctx, systemPrompt, messages, tools)
		if err != nil {
			b.logger.WithError(err).Error("[BEAT] Anthropic API error")
			return
		}

		// Append Claude's response to the conversation
		messages = append(messages, resp.ToParam())

		// Process blocks (text + tool_calls)
		var toolResults []anthropic.ContentBlockParamUnion
		hasToolCall := false

		for _, block := range resp.Content {
			if block.Type == "text" {
				b.logger.WithField("ai_thought", block.Text).Info("[BEAT] AI Log")
			} else if block.Type == "tool_use" {
				hasToolCall = true
				b.logger.WithField("tool", block.Name).Info("[BEAT] AI executing tool")
				
				var argsBytes []byte
				if block.Input != nil {
					argsBytes = block.Input
				}
				
				resStr, toolErr := HandleToolCall(ctx, block.Name, argsBytes, b.data, b.pm, b.trading, b.client)
				
				var resultParam anthropic.ContentBlockParamUnion
				if toolErr != nil {
					resultParam = anthropic.NewToolResultBlock(block.ID, fmt.Sprintf("Error: %s", toolErr.Error()), true)
					b.logger.WithError(toolErr).WithField("tool", block.Name).Warn("[BEAT] Tool failed")
				} else {
					resultParam = anthropic.NewToolResultBlock(block.ID, resStr, false)
				}
				toolResults = append(toolResults, resultParam)
			}
		}

		if !hasToolCall {
			// Claude is done
			b.logger.Info("[BEAT] AI Heartbeat complete")
			break
		}

		// Send tool results back to Claude
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}
