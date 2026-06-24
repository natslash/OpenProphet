package controllers

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"prophet-trader/configstore"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

type DashboardController struct {
	beat   *services.AutonomousBeat
	order  *OrderController
	intent *IntentController
	startTime time.Time
}

func NewDashboardController(beat *services.AutonomousBeat, order *OrderController, intent *IntentController) *DashboardController {
	return &DashboardController{
		beat:      beat,
		order:     order,
		intent:    intent,
		startTime: time.Now(),
	}
}

// HandleSSEEvents is the main SSE endpoint for the dashboard (/api/events).
func (dc *DashboardController) HandleSSEEvents(c *gin.Context) {
	if services.Hub == nil {
		c.JSON(500, gin.H{"error": "SSE hub not initialized"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial state
	state := dc.buildState()
	writeSSE(c, "state", state)
	writeSSE(c, "status", gin.H{"status": statusStr(state["running"])})
	writeSSE(c, "config", configstore.Get())
	c.Writer.Flush()

	ch := services.Hub.Subscribe()
	defer services.Hub.Unsubscribe(ch)

	ctx := c.Request.Context()

	// Periodic state poll (every 5s)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Event, string(evt.Data)))
			c.Writer.Flush()
		case <-ticker.C:
			writeSSE(c, "state", dc.buildState())
			c.Writer.Flush()
		}
	}
}

func (dc *DashboardController) buildState() gin.H {
	cfg := configstore.Get()
	agent := configstore.GetActiveAgent()
	activeModel := cfg.ActiveModel
	if agent != nil && agent.Model != "" {
		activeModel = agent.Model
	}
	return gin.H{
		"running":          dc.beat.IsRunning(),
		"paused":           dc.beat.IsPaused(),
		"heartbeatSeconds": int(dc.beat.Interval().Seconds()),
		"beatCount":        dc.beat.BeatCount(),
		"stats":            dc.beat.Stats(),
		"activeModel":      activeModel,
		"tradingMode":      os.Getenv("TRADING_MODE"),
	}
}

func statusStr(running interface{}) string {
	if r, ok := running.(bool); ok && r {
		return "active"
	}
	return "stopped"
}

func writeSSE(c *gin.Context, event string, data interface{}) {
	c.SSEvent(event, data)
}

// HandleMessage handles chat commands and forwards to the beat controller.
func (dc *DashboardController) HandleMessage(c *gin.Context) {
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "Message is required"})
		return
	}

	trimmed := strings.TrimSpace(req.Message)
	lower := strings.ToLower(trimmed)

	switch {
	case lower == "/help" || lower == "/?":
		cfg := configstore.Get()
		providers := make(map[string]bool)
		for _, m := range cfg.Models {
			parts := strings.SplitN(m.ID, "/", 2)
			if len(parts) > 0 {
				providers[parts[0]] = true
			}
		}
		providerList := make([]string, 0, len(providers))
		for p := range providers {
			providerList = append(providerList, p)
		}
		help := fmt.Sprintf(`Available commands:

/start - Start the agent heartbeat
/stop - Stop the agent heartbeat
/newagent - Create a new agent
/editagent <id> - Edit an existing agent
/agents - List all agents
/portfolios - Show portfolio and positions

Models: %d available
Providers: %s

Any other text will be sent to the agent.`, len(cfg.Models), strings.Join(providerList, ", "))
		c.JSON(200, gin.H{"ok": true, "text": help})
		return

	case lower == "/stop":
		if dc.beat.IsRunning() {
			dc.beat.Stop()
		}
		c.JSON(200, gin.H{"ok": true, "text": "Agent stopped manually."})
		return

	case lower == "/start":
		if !dc.beat.IsRunning() {
			dc.beat.Start()
		}
		c.JSON(200, gin.H{"ok": true, "text": "Agent started."})
		return

	case lower == "/newagent" || strings.HasPrefix(lower, "/newagent "):
		cfg := configstore.Get()
		if services.Hub != nil {
			services.Hub.Broadcast("agent_builder", gin.H{
				"mode":       "create",
				"models":     cfg.Models,
				"strategies": cfg.Strategies,
			})
		}
		c.JSON(200, gin.H{"ok": true, "builder": true})
		return

	case strings.HasPrefix(lower, "/editagent "):
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			c.JSON(400, gin.H{"error": "Usage: /editagent <id>"})
			return
		}
		agentID := parts[1]
		agent := configstore.GetAgentByID(agentID)
		if agent == nil {
			c.JSON(404, gin.H{"error": "Agent not found"})
			return
		}
		cfg := configstore.Get()
		if services.Hub != nil {
			services.Hub.Broadcast("agent_builder", gin.H{
				"mode":       "edit",
				"agent":      agent,
				"models":     cfg.Models,
				"strategies": cfg.Strategies,
			})
		}
		c.JSON(200, gin.H{"ok": true, "builder": true})
		return

	case lower == "/agents":
		cfg := configstore.Get()
		var sb strings.Builder
		sb.WriteString("Available agents:\n")
		for _, a := range cfg.Agents {
			sb.WriteString(fmt.Sprintf("\n- %s (%s)\n  Model: %s\n  Strategy: %s\n",
				a.Name, a.ID,
				orDefault(a.Model, "default"),
				orDefault(derefStr(a.StrategyID), "none")))
		}
		sb.WriteString("\nUse /editagent <id> to edit an agent")
		c.JSON(200, gin.H{"ok": true, "text": sb.String()})
		return

	case lower == "/portfolio" || lower == "/portfolios":
		acc, accErr := dc.order.GetAccount()
		pos, posErr := dc.order.GetPositions()
		var sb strings.Builder
		if accErr == nil {
			sb.WriteString(fmt.Sprintf("Portfolio Status (Account: %s)\n", acc.ID))
			sb.WriteString(fmt.Sprintf("Net Liquidation: €%.2f\n", acc.PortfolioValue))
			sb.WriteString(fmt.Sprintf("Available Cash: €%.2f\n", acc.Cash))
			sb.WriteString(fmt.Sprintf("Buying Power: €%.2f\n\n", acc.BuyingPower))
		} else {
			sb.WriteString("Failed to fetch account data\n\n")
		}
		sb.WriteString("Open Positions:\n")
		if posErr != nil || len(pos) == 0 {
			sb.WriteString("No open positions.\n")
		} else {
			for _, p := range pos {
				sb.WriteString(fmt.Sprintf("- %s: %.0f shares @ €%.2f\n", p.Symbol, p.Qty, p.AvgEntryPrice))
			}
		}
		c.JSON(200, gin.H{"ok": true, "text": sb.String()})
		return
	}

	// Default: forward to beat controller
	if !dc.beat.IsRunning() {
		c.JSON(400, gin.H{"error": "Agent is not running. Please start the agent first."})
		return
	}
	dc.beat.InjectDirectMessage(trimmed)
	c.JSON(200, gin.H{"ok": true, "text": "Instruction sent to autonomous agent."})
}

// HandleHeartbeat sets an override interval for the current heartbeat phase.
func (dc *DashboardController) HandleHeartbeat(c *gin.Context) {
	var req struct {
		Seconds int    `json:"seconds"`
		Reason  string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Seconds < 30 || req.Seconds > 3600 {
		c.JSON(400, gin.H{"error": "seconds must be 30-3600"})
		return
	}
	dc.beat.SetNextInterval(time.Duration(req.Seconds)*time.Second, orDefault(req.Reason, "dashboard override"))
	c.JSON(200, gin.H{"ok": true, "seconds": req.Seconds})
}

// HandlePromptPreview builds the system prompt for the active agent.
func (dc *DashboardController) HandlePromptPreview(c *gin.Context) {
	agent := configstore.GetActiveAgent()
	if agent == nil {
		c.JSON(200, gin.H{"prompt": "No agent configured.", "agentName": ""})
		return
	}
	prompt := agent.CustomSystemPrompt
	if agent.StrategyID != nil {
		strategy := configstore.GetStrategyByID(*agent.StrategyID)
		if strategy != nil && strategy.CustomRules != nil && *strategy.CustomRules != "" {
			prompt += "\n\n## Strategy Rules\n" + *strategy.CustomRules
		}
	}
	prompt += "\n\nCRITICAL CONTEXT:\n- Timezone: CET (Central European Time)\n- Base Currency: EUR (€)\nEnsure all price values, portfolio calculations, and temporal reasoning naturally default to Euros and CET without requiring manual prompting."
	c.JSON(200, gin.H{"prompt": prompt, "agentName": agent.Name})
}

// HandleGetEnv reads environment configuration from .env file.
func (dc *DashboardController) HandleGetEnv(c *gin.Context) {
	env := readEnvFile()
	tradingMode := env["TRADING_MODE"]
	if tradingMode == "" {
		enabled := env["TRADING_ENABLED"] == "true"
		confirm := env["REQUIRE_DOUBLE_CONFIRM"] != "false"
		if !enabled {
			tradingMode = "off"
		} else if confirm {
			tradingMode = "supervised"
		} else {
			tradingMode = "autonomous"
		}
	}
	c.JSON(200, gin.H{
		"LLM_POLLING_ENABLED":      env["LLM_POLLING_ENABLED"] == "true",
		"LLM_POLLING_INTERVAL_SECS": atoi(env["LLM_POLLING_INTERVAL_SECS"], 3600),
		"LLM_PROVIDER":              orDefault(env["LLM_PROVIDER"], "anthropic"),
		"LLM_MODEL":                 env["LLM_MODEL"],
		"BEAT_ENABLED":              env["BEAT_ENABLED"] == "true",
		"TRADING_MODE":              tradingMode,
	})
}

// HandlePostEnv updates environment variables in .env file.
func (dc *DashboardController) HandlePostEnv(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.BindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	envUpdates := make(map[string]string)
	for k, v := range updates {
		switch k {
		case "LLM_POLLING_ENABLED", "BEAT_ENABLED":
			if b, ok := v.(bool); ok {
				if b {
					envUpdates[k] = "true"
				} else {
					envUpdates[k] = "false"
				}
			}
		case "LLM_POLLING_INTERVAL_SECS":
			if n, ok := v.(float64); ok {
				envUpdates[k] = fmt.Sprintf("%d", int(n))
			}
		case "LLM_PROVIDER", "LLM_MODEL", "TRADING_MODE":
			if s, ok := v.(string); ok {
				envUpdates[k] = s
			}
		}
	}

	writeEnvFile(envUpdates)
	for k, v := range envUpdates {
		os.Setenv(k, v)
	}
	c.JSON(200, gin.H{"ok": true})
}

// HandleHealth returns system health.
func (dc *DashboardController) HandleHealth(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "healthy",
		"uptime":  int(time.Since(dc.startTime).Seconds()),
		"running": dc.beat.IsRunning(),
	})
}

// HandleAuthStatus is a stub — auth is managed externally.
func (dc *DashboardController) HandleAuthStatus(c *gin.Context) {
	c.JSON(200, gin.H{"loggedIn": true, "authMethod": "external", "provider": "configured"})
}

// HandleAuthLogin / HandleAuthLogout are no-ops.
func (dc *DashboardController) HandleAuthLogin(c *gin.Context)  { c.JSON(200, gin.H{"ok": true}) }
func (dc *DashboardController) HandleAuthLogout(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) }

// HandleBackendRestart is a no-op — Go serves itself.
func (dc *DashboardController) HandleBackendRestart(c *gin.Context) {
	c.JSON(200, gin.H{"ok": true, "message": "Go backend serves itself; restart the process manually if needed."})
}

// --- Helpers ---

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n == 0 {
		return def
	}
	return n
}

func readEnvFile() map[string]string {
	result := make(map[string]string)
	f, err := os.Open(".env")
	if err != nil {
		return result
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = val
		}
	}
	return result
}

func writeEnvFile(updates map[string]string) {
	current := readEnvFile()
	for k, v := range updates {
		current[k] = v
	}
	var sb strings.Builder
	for k, v := range current {
		sb.WriteString(k + "=" + v + "\n")
	}
	os.WriteFile(".env", []byte(sb.String()), 0644)
}
