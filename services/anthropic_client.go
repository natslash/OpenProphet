package services

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicClient struct {
	client *anthropic.Client
	model  string
}

func NewAnthropicClient() (*AnthropicClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AnthropicClient{
		client: &client,
		model:  "claude-sonnet-4-5",
	}, nil
}

func (ac *AnthropicClient) ExecuteAgentTurn(ctx context.Context, systemPrompt string, messages []anthropic.MessageParam, tools []anthropic.ToolUnionParam) (*anthropic.Message, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(ac.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  messages,
	}

	if len(tools) > 0 {
		params.Tools = tools
	}

	return ac.client.Messages.New(ctx, params)
}
