package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"prophet-trader/interfaces"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
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

	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gemini-1.5-pro"
	}

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (gc *GeminiClient) GetName() string {
	return "Google Gemini 1.5 Pro"
}

func (gc *GeminiClient) GenerateResponse(ctx context.Context, messages []interfaces.LLMMessage, tools []interfaces.LLMTool) (*interfaces.LLMResponse, error) {
	model := gc.client.GenerativeModel(gc.model)

	var history []*genai.Content
	var systemInstruction *genai.Content

	for _, msg := range messages {
		if msg.Role == "system" {
			systemInstruction = &genai.Content{
				Role:  "user", // Gemini uses user role for instructions sometimes, but newer versions support SystemInstruction
				Parts: []genai.Part{genai.Text(msg.Content)},
			}
			model.SystemInstruction = systemInstruction
		} else if msg.ToolResultID != "" {
			history = append(history, &genai.Content{
				Role: "user",
				Parts: []genai.Part{genai.Text(fmt.Sprintf("Tool Result for '%s':\n%s", msg.ToolResultID, msg.Content))},
			})
		} else {
			role := "user"
			var parts []genai.Part
			if msg.Role == "assistant" {
				role = "model"
				if msg.Content != "" {
					parts = append(parts, genai.Text(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					parts = append(parts, genai.Text(fmt.Sprintf("[Executed Tool: %s]\nArguments: %s", tc.Name, string(tc.Arguments))))
				}
			} else {
				parts = append(parts, genai.Text(msg.Content))
			}

			// Do not append empty messages to history
			if len(parts) > 0 {
				history = append(history, &genai.Content{
					Role:  role,
					Parts: parts,
				})
			}
		}
	}

	// Register tools
	if len(tools) > 0 {
		var genaiTools []*genai.Tool
		var functionDeclarations []*genai.FunctionDeclaration

		for _, tool := range tools {
			// Convert input schema (map) to genai.Schema
			schema := convertGenaiSchema(tool.InputSchema)

			functionDeclarations = append(functionDeclarations, &genai.FunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  schema,
			})
		}

		genaiTools = append(genaiTools, &genai.Tool{
			FunctionDeclarations: functionDeclarations,
		})
		model.Tools = genaiTools
	}

	// Send request
	var resp *genai.GenerateContentResponse
	var err error

	if len(history) == 0 {
		return nil, fmt.Errorf("no user messages provided")
	}

	maxRetries := 3
	for i := 0; i <= maxRetries; i++ {
		if len(history) == 1 {
			resp, err = model.GenerateContent(ctx, history[0].Parts...)
		} else {
			session := model.StartChat()
			// Feed all but last message to history
			session.History = history[:len(history)-1]
			resp, err = session.SendMessage(ctx, history[len(history)-1].Parts...)
		}

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

	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			content += string(text)
		} else if funcCall, ok := part.(genai.FunctionCall); ok {
			argsBytes, _ := json.Marshal(funcCall.Args)
			toolCalls = append(toolCalls, interfaces.LLMToolCall{
				ID:        funcCall.Name, // Gemini doesn't use unique IDs for tool calls in the same way, we use the name as ID
				Name:      funcCall.Name,
				Arguments: argsBytes,
			})
		}
	}

	usage := 0
	if resp.UsageMetadata != nil {
		usage = int(resp.UsageMetadata.TotalTokenCount)
	}

	return &interfaces.LLMResponse{
		Content:    content,
		ToolCalls:  toolCalls,
		UsageToken: usage,
	}, nil
}

func convertGenaiSchema(input map[string]interface{}) *genai.Schema {
	if input == nil {
		return nil
	}
	
	schema := &genai.Schema{}
	
	if t, ok := input["type"].(string); ok {
		switch t {
		case "object": schema.Type = genai.TypeObject
		case "string": schema.Type = genai.TypeString
		case "integer": schema.Type = genai.TypeInteger
		case "number": schema.Type = genai.TypeNumber
		case "boolean": schema.Type = genai.TypeBoolean
		case "array": schema.Type = genai.TypeArray
		}
	}
	
	if desc, ok := input["description"].(string); ok {
		schema.Description = desc
	}
	
	if req, ok := input["required"].([]interface{}); ok {
		for _, r := range req {
			if rs, ok := r.(string); ok {
				schema.Required = append(schema.Required, rs)
			}
		}
	} else if req, ok := input["required"].([]string); ok {
		schema.Required = req
	}
	
	if props, ok := input["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for k, v := range props {
			if vMap, ok := v.(map[string]interface{}); ok {
				schema.Properties[k] = convertGenaiSchema(vMap)
			}
		}
	}
	
	if items, ok := input["items"].(map[string]interface{}); ok {
		schema.Items = convertGenaiSchema(items)
	}

	if enums, ok := input["enum"].([]interface{}); ok {
		for _, e := range enums {
			if es, ok := e.(string); ok {
				schema.Enum = append(schema.Enum, es)
			}
		}
	} else if enums, ok := input["enum"].([]string); ok {
		schema.Enum = enums
	}
	
	return schema
}
