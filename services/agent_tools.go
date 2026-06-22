package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"prophet-trader/interfaces"
)

func BuildAgentTools() []interfaces.LLMTool {
	return []interfaces.LLMTool{
		{
			Name:        "get_account",
			Description: "Get the current account status including BuyingPower, Cash, and NetLiquidation",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_positions",
			Description: "Get all open stock positions",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "get_options_positions",
			Description: "Get all open options positions",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "place_managed_position",
			Description: "Place a new managed stock position. side must be 'buy' or 'sell'. EntryStrategy must be 'limit' or 'market'. EntryPrice is optional for limit.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol":              map[string]interface{}{"type": "string", "description": "e.g. AAPL"},
					"side":                map[string]interface{}{"type": "string", "enum": []string{"buy", "sell"}},
					"strategy":            map[string]interface{}{"type": "string", "enum": []string{"DAY_TRADE", "SWING_TRADE"}},
					"entry_strategy":      map[string]interface{}{"type": "string", "enum": []string{"limit", "market"}},
					"entry_price":         map[string]interface{}{"type": "number"},
					"stop_loss_percent":   map[string]interface{}{"type": "number"},
					"take_profit_percent": map[string]interface{}{"type": "number"},
					"notes":               map[string]interface{}{"type": "string"},
					"explicit_quantity":   map[string]interface{}{"type": "integer"},
				},
				"required": []string{"symbol", "side", "strategy", "entry_strategy", "explicit_quantity"},
			},
		},
		{
			Name:        "get_quote",
			Description: "Get the current live or latest market quote for a specific symbol (e.g. 'ESTX50', 'AAPL')",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol": map[string]interface{}{"type": "string"},
				},
				"required": []string{"symbol"},
			},
		},
		{
			Name:        "get_options_chain",
			Description: "Get the options chain (available strikes and expirations) for an underlying symbol",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol":     map[string]interface{}{"type": "string", "description": "e.g. 'ESTX50'"},
					"expiration": map[string]interface{}{"type": "string", "description": "e.g. '20260816' (YYYYMMDD)"},
				},
				"required": []string{"symbol", "expiration"},
			},
		},
		{
			Name:        "place_options_order",
			Description: "Place a new options order. Symbol must be formatted like ESTX50:20260619:C:6325",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol":     map[string]interface{}{"type": "string", "description": "e.g. ESTX50:20260619:C:6325"},
					"action":     map[string]interface{}{"type": "string", "enum": []string{"BUY", "SELL"}},
					"qty":        map[string]interface{}{"type": "integer"},
					"order_type": map[string]interface{}{"type": "string", "enum": []string{"MKT", "LMT", "MIDPRICE"}},
					"lmt_price":  map[string]interface{}{"type": "number"},
				},
				"required": []string{"symbol", "action", "qty", "order_type"},
			},
		},
		{
			Name:        "jim_rogers",
			Description: "Consult another agent (e.g. 'stratagem' or 'daedalus') to analyze data or review risk. They will return a text response.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target_agent_id": map[string]interface{}{"type": "string"},
					"prompt":          map[string]interface{}{"type": "string"},
				},
				"required": []string{"target_agent_id", "prompt"},
			},
		},
	}
}

func ExecuteAgentTool(ctx context.Context, toolName string, args []byte, data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService, llm interfaces.LLMProvider, intentManager *IntentManager, requireDoubleConfirm bool) (string, error) {
	return HandleToolCall(ctx, toolName, args, data, pm, trading, llm, intentManager, requireDoubleConfirm)
}

// HandleToolCall executes the local method and returns the JSON string result
func HandleToolCall(ctx context.Context, toolName string, args []byte, data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService, llm interfaces.LLMProvider, intentManager *IntentManager, requireDoubleConfirm bool) (string, error) {
	switch toolName {
	case "get_account":
		acc, err := trading.GetAccount(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(acc)
		return string(b), nil

	case "get_positions":
		pos, err := trading.GetPositions(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(pos)
		return string(b), nil

	case "get_options_positions":
		pos, err := trading.ListOptionsPositions(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(pos)
		return string(b), nil

	case "place_managed_position":
		var req PlaceManagedPositionRequest
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
		
		if requireDoubleConfirm && intentManager != nil {
			currentPrice := 0.0
			if data != nil {
				if quote, err := data.GetLatestQuote(ctx, req.Symbol); err == nil && quote != nil {
					if quote.AskPrice > 0 {
						currentPrice = quote.AskPrice
					} else if quote.BidPrice > 0 {
						currentPrice = quote.BidPrice
					}
				}
			}
			
			qty := 0.0
			if req.ExplicitQuantity != nil {
				qty = float64(*req.ExplicitQuantity)
			} else if currentPrice > 0 {
				qty = pm.resolveQuantity(&req, currentPrice)
			}
			
			id, err := intentManager.CreateIntent(IntentTypeManagedPosition, args, req.Symbol, req.Side, qty, currentPrice)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Order intent created and pending human authorization (Intent ID: %s). Do not retry. The user will review your proposed trade.", id), nil
		}

		pos, err := pm.PlaceManagedPosition(ctx, &req)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(pos)
		return string(b), nil

	case "place_options_order":
		var req struct {
			Symbol    string  `json:"symbol"`
			Action    string  `json:"action"`
			Qty       float64 `json:"qty"`
			OrderType string  `json:"order_type"`
			LmtPrice  float64 `json:"lmt_price"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}

		var lmtPricePtr *float64
		if req.OrderType == "LMT" {
			lmtPricePtr = &req.LmtPrice
		}

		order := &interfaces.OptionsOrder{
			Symbol:     req.Symbol,
			Qty:        req.Qty,
			Side:       req.Action,
			Type:       req.OrderType,
			LimitPrice: lmtPricePtr,
		}
		
		if requireDoubleConfirm && intentManager != nil {
			currentPrice := 0.0
			if data != nil {
				if quote, err := data.GetLatestQuote(ctx, req.Symbol); err == nil && quote != nil {
					if quote.AskPrice > 0 {
						currentPrice = quote.AskPrice
					} else if quote.BidPrice > 0 {
						currentPrice = quote.BidPrice
					}
				}
			}
			id, err := intentManager.CreateIntent(IntentTypeOptionsOrder, args, req.Symbol, req.Action, req.Qty, currentPrice)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Options order intent created and pending human authorization (Intent ID: %s). Do not retry. The user will review your proposed trade.", id), nil
		}

		res, err := trading.PlaceOptionsOrder(ctx, order)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"order_id": "%s"}`, res.OrderID), nil

	case "get_quote":
		var req struct {
			Symbol string `json:"symbol"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
		if data == nil {
			return "", fmt.Errorf("data service not initialized")
		}
		quote, err := data.GetLatestQuote(ctx, req.Symbol)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(quote)
		return string(b), nil

	case "get_options_chain":
		var req struct {
			Symbol     string `json:"symbol"`
			Expiration string `json:"expiration"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
		if trading == nil {
			return "", fmt.Errorf("trading service not initialized")
		}
		// Try to parse expiration
		exp, err := time.Parse("20060102", req.Expiration)
		if err != nil {
			// fallback, just use now
			exp = time.Now().Add(45 * 24 * time.Hour)
		}
		chain, err := trading.GetOptionsChain(ctx, req.Symbol, exp)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(chain)
		return string(b), nil

	case "jim_rogers":
		var req struct {
			TargetAgentID string `json:"target_agent_id"`
			Prompt        string `json:"prompt"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}

		configData, err := os.ReadFile("data/agent-config.json")
		if err != nil {
			return "", fmt.Errorf("failed to read agent config: %v", err)
		}
		var cfg struct {
			Agents []struct {
				ID                 string `json:"id"`
				CustomSystemPrompt string `json:"customSystemPrompt"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(configData, &cfg); err != nil {
			return "", fmt.Errorf("failed to parse agent config: %v", err)
		}
		var sysPrompt string
		for _, a := range cfg.Agents {
			if a.ID == req.TargetAgentID {
				sysPrompt = a.CustomSystemPrompt
				break
			}
		}
		if sysPrompt == "" {
			return "", fmt.Errorf("agent %s not found in config", req.TargetAgentID)
		}

		if llm == nil {
			return "", fmt.Errorf("llm provider not initialized for jim_rogers tool")
		}

		messages := []interfaces.LLMMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: req.Prompt},
		}

		resp, err := llm.GenerateResponse(ctx, messages, nil)
		if err != nil {
			return "", fmt.Errorf("failed to consult agent: %v", err)
		}
		
		return resp.Content, nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
