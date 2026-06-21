package interfaces

import "context"

// LLMMessage represents a message in a conversation
type LLMMessage struct {
	Role         string // "user", "assistant", "system"
	Content      string
	ToolResultID string // If set, this message is a tool result
}

// LLMTool represents a callable tool/function for the LLM
type LLMTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// LLMToolCall represents the LLM requesting to call a tool
type LLMToolCall struct {
	ID        string
	Name      string
	Arguments []byte // JSON encoded arguments
}

// LLMToolResult represents the outcome of a tool execution
type LLMToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// LLMResponse represents the standard output from any LLM provider
type LLMResponse struct {
	Content    string
	ToolCalls  []LLMToolCall
	UsageToken int
}

// LLMProvider defines the interface for any AI backend (Anthropic, OpenAI, Local, etc.)
type LLMProvider interface {
	// GenerateResponse sends a conversation to the LLM and gets a response, optionally with tools
	GenerateResponse(ctx context.Context, messages []LLMMessage, tools []LLMTool) (*LLMResponse, error)
	
	// GetName returns the display name of the provider (e.g. "Anthropic Claude 3.5 Sonnet")
	GetName() string
}
