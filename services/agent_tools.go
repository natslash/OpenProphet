package services

import (
	"context"
	"encoding/json"
	"fmt"
	"prophet-trader/interfaces"
	"github.com/anthropics/anthropic-sdk-go"
)

func BuildAgentTools() []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_account",
				Description: anthropic.String("Get the current account status including BuyingPower, Cash, and NetLiquidation"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_positions",
				Description: anthropic.String("Get all open stock positions"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "get_options_positions",
				Description: anthropic.String("Get all open options positions"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "place_managed_position",
				Description: anthropic.String("Place a new managed stock position. side must be 'buy' or 'sell'. EntryStrategy must be 'limit' or 'market'. EntryPrice is optional for limit."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"symbol":              map[string]interface{}{"type": "string", "description": "e.g. AAPL"},
						"side":                map[string]interface{}{"type": "string", "enum": []string{"buy", "sell"}},
						"strategy":            map[string]interface{}{"type": "string", "enum": []string{"DAY_TRADE", "SWING_TRADE"}},
						"entry_strategy":      map[string]interface{}{"type": "string", "enum": []string{"limit", "market"}},
						"entry_price":         map[string]interface{}{"type": "number"},
						"stop_loss_percent":   map[string]interface{}{"type": "number"},
						"take_profit_percent": map[string]interface{}{"type": "number"},
						"notes":               map[string]interface{}{"type": "string"},
						"quantity":            map[string]interface{}{"type": "integer"},
					},
					Required: []string{"symbol", "side", "strategy", "entry_strategy", "quantity"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "place_options_order",
				Description: anthropic.String("Place a new options order. Symbol must be formatted like ESTX50:20260619:C:6325"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]interface{}{
						"symbol": map[string]interface{}{"type": "string", "description": "e.g. ESTX50:20260619:C:6325"},
						"action": map[string]interface{}{"type": "string", "enum": []string{"BUY", "SELL"}},
						"qty":    map[string]interface{}{"type": "integer"},
						"order_type": map[string]interface{}{"type": "string", "enum": []string{"MKT", "LMT", "MIDPRICE"}},
						"lmt_price": map[string]interface{}{"type": "number"},
					},
					Required: []string{"symbol", "action", "qty", "order_type"},
				},
			},
		},
	}
}

// HandleToolCall executes the local method and returns the JSON string result
func HandleToolCall(ctx context.Context, toolName string, args []byte, data interfaces.DataService, pm *PositionManager, trading interfaces.TradingService) (string, error) {
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
			Symbol: req.Symbol,
			Qty:    req.Qty,
			Side:   req.Action,
			Type:   req.OrderType,
			LimitPrice: lmtPricePtr,
		}
		res, err := trading.PlaceOptionsOrder(ctx, order)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"order_id": "%s"}`, res.OrderID), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
