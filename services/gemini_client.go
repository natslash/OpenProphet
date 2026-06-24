package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"prophet-trader/interfaces"

	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
}

func NewGeminiClient() (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (gc *GeminiClient) GetName() string {
	return "Google Gemini (" + gc.model + ")"
}

func (gc *GeminiClient) GenerateResponse(ctx context.Context, messages []interfaces.LLMMessage, tools []interfaces.LLMTool) (*interfaces.LLMResponse, error) {
	config := &genai.GenerateContentConfig{}

	var contents []*genai.Content

	var pendingFuncParts []*genai.Part

	flushFuncResults := func() {
		if len(pendingFuncParts) > 0 {
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: pendingFuncParts,
			})
			pendingFuncParts = nil
		}
	}

	for _, msg := range messages {
		if msg.Role == "system" {
			config.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{{Text: msg.Content}},
			}
		} else if msg.ToolResultID != "" {
			funcName := msg.ToolResultName
			if funcName == "" {
				funcName = msg.ToolResultID
			}
			var resultMap map[string]any
			if err := json.Unmarshal([]byte(msg.Content), &resultMap); err != nil {
				resultMap = map[string]any{"result": msg.Content}
			}
			pendingFuncParts = append(pendingFuncParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     funcName,
					Response: resultMap,
				},
			})
		} else {
			flushFuncResults()
			if msg.Role == "assistant" {
				if raw, ok := msg.RawContent.(*genai.Content); ok && raw != nil {
					contents = append(contents, raw)
				} else {
					var parts []*genai.Part
					if msg.Content != "" {
						parts = append(parts, &genai.Part{Text: msg.Content})
					}
					for _, tc := range msg.ToolCalls {
						var args map[string]any
						if err := json.Unmarshal(tc.Arguments, &args); err != nil {
							args = map[string]any{}
						}
						parts = append(parts, &genai.Part{
							FunctionCall: &genai.FunctionCall{
								Name: tc.Name,
								Args: args,
							},
						})
					}
					if len(parts) > 0 {
						contents = append(contents, &genai.Content{Role: "model", Parts: parts})
					}
				}
			} else {
				contents = append(contents, &genai.Content{
					Role:  "user",
					Parts: []*genai.Part{{Text: msg.Content}},
				})
			}
		}
	}
	flushFuncResults()

	if len(tools) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, tool := range tools {
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 tool.Name,
				Description:          tool.Description,
				ParametersJsonSchema: tool.InputSchema,
			})
		}
		config.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
	}

	if len(contents) == 0 {
		return nil, fmt.Errorf("no user messages provided")
	}

	var resp *genai.GenerateContentResponse
	var err error

	maxRetries := 3
	for i := 0; i <= maxRetries; i++ {
		resp, err = gc.client.Models.GenerateContent(ctx, gc.model, contents, config)
		if err == nil {
			break
		}
		if i < maxRetries && (strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "429")) {
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}
		return nil, err
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no response generated")
	}

	var content string
	var toolCalls []interfaces.LLMToolCall

	for i, part := range resp.Candidates[0].Content.Parts {
		if part.Text != "" && !part.Thought {
			content += part.Text
		} else if part.FunctionCall != nil {
			argsBytes, _ := json.Marshal(part.FunctionCall.Args)
			toolCalls = append(toolCalls, interfaces.LLMToolCall{
				ID:        fmt.Sprintf("%s_%d", part.FunctionCall.Name, i),
				Name:      part.FunctionCall.Name,
				Arguments: argsBytes,
			})
		}
	}

	usage := 0
	if resp.UsageMetadata != nil {
		usage = int(resp.UsageMetadata.TotalTokenCount)
		if resp.UsageMetadata.CachedContentTokenCount > 0 {
			fmt.Printf("[GEMINI] Implicit caching active! Cached tokens: %d / Total prompt: %d\n",
				resp.UsageMetadata.CachedContentTokenCount,
				resp.UsageMetadata.PromptTokenCount)
		}
	}

	return &interfaces.LLMResponse{
		Content:    content,
		ToolCalls:  toolCalls,
		UsageToken: usage,
		RawContent: resp.Candidates[0].Content,
	}, nil
}
