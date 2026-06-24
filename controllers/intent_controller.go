package controllers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"prophet-trader/config"
	"prophet-trader/interfaces"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

type IntentController struct {
	intentManager *services.IntentManager
	pm            *services.PositionManager
	trading       interfaces.TradingService
	data          interfaces.DataService
	logger        *services.ActivityLogger
}

func NewIntentController(im *services.IntentManager, pm *services.PositionManager, trading interfaces.TradingService, data interfaces.DataService, logger *services.ActivityLogger) *IntentController {
	return &IntentController{
		intentManager: im,
		pm:            pm,
		trading:       trading,
		data:          data,
		logger:        logger,
	}
}

func (ic *IntentController) requireAdminToken(c *gin.Context) bool {
	authHeader := c.GetHeader("Authorization")
	expectedToken := config.AppConfig.AdminToken

	if expectedToken == "" {
		c.JSON(403, gin.H{"error": "Authorization not configured. Live execution is disabled."})
		return false
	}

	const prefix = "Bearer "
	if len(authHeader) < len(prefix) || authHeader[:len(prefix)] != prefix {
		c.JSON(401, gin.H{"error": "Invalid authorization header format"})
		return false
	}

	token := authHeader[len(prefix):]
	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return false
	}

	return true
}

func (ic *IntentController) HandleGetIntents(c *gin.Context) {
	intents := ic.intentManager.ListIntents()
	c.JSON(200, intents)
}

func (ic *IntentController) HandleGetHistory(c *gin.Context) {
	history := ic.intentManager.ListHistory()
	c.JSON(200, history)
}

func (ic *IntentController) HandleAuthorizeIntent(c *gin.Context) {
	if !ic.requireAdminToken(c) {
		return
	}

	id := c.Param("id")
	intent, err := ic.intentManager.ClaimForExecution(id)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// No reference price was captured at intent creation (e.g. the quote timed
	// out off-hours), so slippage cannot be validated. Fail closed on the live
	// port; on paper, proceed but record that the order was not price-validated
	// so the skipped guard is never silent.
	if intent.CurrentPrice <= 0 {
		if config.AppConfig.IBKRPort == 4001 {
			reason := "no reference price captured at intent creation; cannot validate slippage on live — re-submit when quotes are available"
			ic.intentManager.RejectIntent(id, reason)
			c.JSON(400, gin.H{"error": reason})
			return
		}
		if ic.logger != nil {
			ic.logger.LogActivity("SYSTEM", "Intent Authorized (UNVALIDATED PRICE)", intent.Symbol,
				"No reference price captured at intent creation; slippage guard skipped (paper)",
				map[string]interface{}{"intent_id": id})
		}
	}

	// Stale price guard
	if ic.data != nil && intent.CurrentPrice > 0 {
		if quote, err := ic.data.GetLatestQuote(c.Request.Context(), intent.Symbol); err == nil && quote != nil {
			var livePrice float64
			if quote.AskPrice > 0 {
				livePrice = quote.AskPrice
			} else if quote.BidPrice > 0 {
				livePrice = quote.BidPrice
			}

			if livePrice > 0 {
				diffPercent := (livePrice - intent.CurrentPrice) / intent.CurrentPrice
				if diffPercent < 0 {
					diffPercent = -diffPercent
				}
				diffPercent = diffPercent * 100

				if diffPercent > config.AppConfig.MaxPriceSlippagePercent {
					reason := fmt.Sprintf("Stale price guard triggered. Intent price: %.2f, Live price: %.2f (Diff: %.2f%% > Max %.2f%%)",
						intent.CurrentPrice, livePrice, diffPercent, config.AppConfig.MaxPriceSlippagePercent)
					
					ic.intentManager.RejectIntent(id, reason)
					c.JSON(400, gin.H{"error": reason})
					return
				}
			}
		}
	}

	// Execute
	var execErr error
	if intent.Type == services.IntentTypeManagedPosition {
		var req services.PlaceManagedPositionRequest
		if err := json.Unmarshal(intent.Payload, &req); err != nil {
			execErr = err
		} else {
			_, execErr = ic.pm.PlaceManagedPosition(c.Request.Context(), &req)
		}
	} else if intent.Type == services.IntentTypeOptionsOrder {
		var probe struct {
			Legs json.RawMessage `json:"legs"`
		}
		json.Unmarshal(intent.Payload, &probe)

		if len(probe.Legs) > 2 {
			var req struct {
				Legs []struct {
					Symbol string `json:"symbol"`
					Action string `json:"action"`
					Ratio  int    `json:"ratio"`
				} `json:"legs"`
				Action    string  `json:"action"`
				Qty       float64 `json:"qty"`
				OrderType string  `json:"order_type"`
				LmtPrice  float64 `json:"lmt_price"`
			}
			if err := json.Unmarshal(intent.Payload, &req); err != nil {
				execErr = err
			} else {
				legs := make([]interfaces.ComboLeg, len(req.Legs))
				for i, l := range req.Legs {
					ratio := l.Ratio
					if ratio <= 0 {
						ratio = 1
					}
					legs[i] = interfaces.ComboLeg{Symbol: l.Symbol, Action: l.Action, Ratio: ratio}
				}
				var lmtPricePtr *float64
				if req.OrderType == "LMT" {
					lmtPricePtr = &req.LmtPrice
				}
				_, execErr = ic.trading.PlaceComboOrder(c.Request.Context(), &interfaces.ComboOrder{
					Legs: legs, Action: req.Action, Qty: req.Qty,
					OrderType: req.OrderType, LimitPrice: lmtPricePtr,
				})
			}
		} else {
			var req struct {
				Symbol    string  `json:"symbol"`
				Action    string  `json:"action"`
				Qty       float64 `json:"qty"`
				OrderType string  `json:"order_type"`
				LmtPrice  float64 `json:"lmt_price"`
			}
			if err := json.Unmarshal(intent.Payload, &req); err != nil {
				execErr = err
			} else {
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
				_, execErr = ic.trading.PlaceOptionsOrder(c.Request.Context(), order)
			}
		}
	}

	if execErr != nil {
		ic.intentManager.RejectIntent(id, fmt.Sprintf("Execution failed: %v", execErr))
		c.JSON(500, gin.H{"error": execErr.Error()})
		return
	}

	// Log Activity
	if ic.logger != nil {
		ic.logger.LogActivity("SYSTEM", "Intent Authorized", intent.Symbol, "Human Authorized", map[string]interface{}{
			"intent_id": id,
			"qty":       intent.Quantity,
			"side":      intent.Side,
		})
	}

	ic.intentManager.MarkCompleted(id)
	c.JSON(200, gin.H{"status": "authorized", "intent_id": id})
}

func (ic *IntentController) HandleRejectIntent(c *gin.Context) {
	if !ic.requireAdminToken(c) {
		return
	}

	id := c.Param("id")
	if err := ic.intentManager.RejectIntent(id, "Rejected by user"); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if ic.logger != nil {
		ic.logger.LogActivity("SYSTEM", "Intent Rejected", "", "Human Rejected", map[string]interface{}{
			"intent_id": id,
		})
	}

	c.JSON(200, gin.H{"status": "rejected", "intent_id": id})
}
