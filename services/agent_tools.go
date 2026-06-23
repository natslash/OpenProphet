package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
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
			Name:        "set_heartbeat",
			Description: "Change the interval until the next heartbeat tick. Use when the user asks to wait, delay, or change timing. The override applies to the next tick only, then reverts to the default interval.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"seconds": map[string]interface{}{"type": "integer", "description": "Seconds until next heartbeat (60-3600)", "minimum": 60, "maximum": 3600},
					"reason":  map[string]interface{}{"type": "string", "description": "Why the interval is being changed"},
				},
				"required": []string{"seconds", "reason"},
			},
		},
		{
			Name:        "search_contract",
			Description: "Search for tradable instruments on IBKR by name or symbol. Returns matching contracts with exchange, currency, and type. Use this to discover symbols you don't know the exact format for.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"symbol":   map[string]interface{}{"type": "string", "description": "Symbol or partial name to search, e.g. 'SPX', 'AAPL', 'ESTX50'"},
					"sec_type": map[string]interface{}{"type": "string", "description": "Security type filter: STK, OPT, FUT, IND. Leave empty for any."},
					"exchange": map[string]interface{}{"type": "string", "description": "Exchange filter, e.g. 'SMART', 'EUREX', 'CBOE'. Leave empty for any."},
					"currency": map[string]interface{}{"type": "string", "description": "Currency filter, e.g. 'USD', 'EUR'. Leave empty for any."},
				},
				"required": []string{"symbol"},
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

type ToolContext struct {
	Data                 interfaces.DataService
	PM                   *PositionManager
	Trading              interfaces.TradingService
	LLM                  interfaces.LLMProvider
	Intent               *IntentManager
	Beat                 *AutonomousBeat
	Resolver             *tws.ContractResolver
	RequireDoubleConfirm bool
}

func ExecuteAgentTool(ctx context.Context, toolName string, args []byte, tc *ToolContext) (string, error) {
	return HandleToolCall(ctx, toolName, args, tc)
}

func HandleToolCall(ctx context.Context, toolName string, args []byte, tc *ToolContext) (string, error) {
	switch toolName {
	case "get_account":
		acc, err := tc.Trading.GetAccount(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(acc)
		return string(b), nil

	case "get_positions":
		pos, err := tc.Trading.GetPositions(ctx)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(pos)
		return string(b), nil

	case "get_options_positions":
		pos, err := tc.Trading.ListOptionsPositions(ctx)
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

		if tc.RequireDoubleConfirm && tc.Intent != nil {
			currentPrice := 0.0
			if tc.Data != nil {
				quoteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				if quote, err := tc.Data.GetLatestQuote(quoteCtx, req.Symbol); err == nil && quote != nil {
					if quote.AskPrice > 0 {
						currentPrice = quote.AskPrice
					} else if quote.BidPrice > 0 {
						currentPrice = quote.BidPrice
					}
				}
				cancel()
			}

			qty := 0.0
			if req.ExplicitQuantity != nil {
				qty = float64(*req.ExplicitQuantity)
			} else if currentPrice > 0 {
				qty = tc.PM.resolveQuantity(&req, currentPrice)
			}

			id, err := tc.Intent.CreateIntent(IntentTypeManagedPosition, args, req.Symbol, req.Side, qty, currentPrice)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Order intent created and pending human authorization (Intent ID: %s). Do not retry. The user will review your proposed trade.", id), nil
		}

		pos, err := tc.PM.PlaceManagedPosition(ctx, &req)
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

		if tc.RequireDoubleConfirm && tc.Intent != nil {
			currentPrice := 0.0
			if tc.Data != nil {
				quoteCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				if quote, err := tc.Data.GetLatestQuote(quoteCtx, req.Symbol); err == nil && quote != nil {
					if quote.AskPrice > 0 {
						currentPrice = quote.AskPrice
					} else if quote.BidPrice > 0 {
						currentPrice = quote.BidPrice
					}
				}
				cancel()
			}
			id, err := tc.Intent.CreateIntent(IntentTypeOptionsOrder, args, req.Symbol, req.Action, req.Qty, currentPrice)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Options order intent created and pending human authorization (Intent ID: %s). Do not retry. The user will review your proposed trade.", id), nil
		}

		res, err := tc.Trading.PlaceOptionsOrder(ctx, order)
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
		if tc.Data == nil {
			return "", fmt.Errorf("data service not initialized")
		}
		quoteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		quote, err := tc.Data.GetLatestQuote(quoteCtx, req.Symbol)
		cancel()
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
		if tc.Trading == nil {
			return "", fmt.Errorf("trading service not initialized")
		}
		exp, err := time.Parse("20060102", req.Expiration)
		if err != nil {
			exp = time.Now().Add(45 * 24 * time.Hour)
		}
		chain, err := tc.Trading.GetOptionsChain(ctx, req.Symbol, exp)
		if err != nil {
			return "", err
		}
		return formatOptionsChain(ctx, tc.Data, req.Symbol, chain), nil

	case "set_heartbeat":
		var req struct {
			Seconds int    `json:"seconds"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
		if tc.Beat == nil {
			return "", fmt.Errorf("heartbeat control not available")
		}
		if req.Seconds < 60 {
			req.Seconds = 60
		}
		if req.Seconds > 3600 {
			req.Seconds = 3600
		}
		tc.Beat.SetNextInterval(time.Duration(req.Seconds)*time.Second, req.Reason)
		return fmt.Sprintf("Next heartbeat interval set to %d seconds. Reason: %s", req.Seconds, req.Reason), nil

	case "search_contract":
		var req struct {
			Symbol   string `json:"symbol"`
			SecType  string `json:"sec_type"`
			Exchange string `json:"exchange"`
			Currency string `json:"currency"`
		}
		if err := json.Unmarshal(args, &req); err != nil {
			return "", err
		}
		if tc.Resolver == nil {
			return "", fmt.Errorf("contract resolver not available")
		}
		searchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		results, err := tc.Resolver.Search(searchCtx, req.Symbol, req.SecType, req.Exchange, req.Currency)
		cancel()
		if err != nil {
			return "", fmt.Errorf("search failed: %w", err)
		}
		type contractResult struct {
			Symbol   string `json:"symbol"`
			SecType  string `json:"sec_type"`
			Exchange string `json:"exchange"`
			Currency string `json:"currency"`
			ConId    int64  `json:"con_id"`
			LongName string `json:"long_name,omitempty"`
		}
		var out []contractResult
		for _, cd := range results {
			out = append(out, contractResult{
				Symbol:   cd.Contract.Symbol,
				SecType:  string(cd.Contract.SecType),
				Exchange: cd.Contract.Exchange,
				Currency: cd.Contract.Currency,
				ConId:    cd.Contract.ConId,
				LongName: cd.LongName,
			})
		}
		if len(out) == 0 {
			return `{"results": [], "message": "No contracts found"}`, nil
		}
		b, _ := json.Marshal(map[string]interface{}{"results": out, "count": len(out)})
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

		if tc.LLM == nil {
			return "", fmt.Errorf("llm provider not initialized for jim_rogers tool")
		}

		messages := []interfaces.LLMMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: req.Prompt},
		}

		resp, err := tc.LLM.GenerateResponse(ctx, messages, nil)
		if err != nil {
			return "", fmt.Errorf("failed to consult agent: %v", err)
		}

		return resp.Content, nil

	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

// compactOption is the token-lean projection of an option contract sent to the
// LLM. We drop fields the agent rarely reasons over (premium, volume, gamma,
// theta, vega, full expiration date, redundant underlying) to cut payload size.
type compactOption struct {
	Sym   string  `json:"sym"`
	Type  string  `json:"type"` // "C" / "P"
	K     float64 `json:"strike"`
	Bid   float64 `json:"bid"`
	Ask   float64 `json:"ask"`
	Delta float64 `json:"delta,omitempty"`
	IV    float64 `json:"iv,omitempty"`
	OI    int64   `json:"oi,omitempty"`
	DTE   int     `json:"dte,omitempty"`
}

// formatOptionsChain projects the chain to compact fields and windows the
// strikes around the underlying spot so the LLM only sees the relevant,
// near-the-money contracts. A full OESX chain is hundreds of contracts with 16
// fields each; raw-marshaled and resent every conversation turn it dominates
// token usage. We fetch spot (best-effort, short timeout) to centre the window;
// if spot is unavailable we fall back to a hard contract cap so the payload
// stays bounded regardless.
func formatOptionsChain(ctx context.Context, data interfaces.DataService, underlying string, chain []*interfaces.OptionContract) string {
	const strikeWindow = 0.15 // keep strikes within ±15% of spot
	const maxContracts = 60   // hard cap used only when spot is unknown

	// Best-effort spot for windowing (don't block the tool if quotes are sparse).
	spot := 0.0
	if data != nil {
		qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if q, err := data.GetLatestQuote(qctx, underlying); err == nil && q != nil {
			switch {
			case q.AskPrice > 0 && q.BidPrice > 0:
				spot = (q.AskPrice + q.BidPrice) / 2
			case q.AskPrice > 0:
				spot = q.AskPrice
			case q.BidPrice > 0:
				spot = q.BidPrice
			}
		}
		cancel()
	}

	out := make([]compactOption, 0, len(chain))
	for _, c := range chain {
		if c == nil {
			continue
		}
		if spot > 0 && (c.StrikePrice < spot*(1-strikeWindow) || c.StrikePrice > spot*(1+strikeWindow)) {
			continue
		}
		t := "C"
		if c.ContractType == "put" || c.ContractType == "P" || c.ContractType == "p" {
			t = "P"
		}
		out = append(out, compactOption{
			Sym: c.Symbol, Type: t, K: c.StrikePrice,
			Bid: c.Bid, Ask: c.Ask, Delta: c.Delta, IV: c.ImpliedVolatility,
			OI: c.OpenInterest, DTE: c.DTE,
		})
	}
	// When spot is unknown we couldn't window; cap the count to bound tokens.
	if spot <= 0 && len(out) > maxContracts {
		out = out[:maxContracts]
	}

	payload := map[string]interface{}{
		"underlying": underlying,
		"spot":       spot,
		"returned":   len(out),
		"total":      len(chain),
		"note":       "near-the-money window; fields projected to conserve context",
		"options":    out,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}
