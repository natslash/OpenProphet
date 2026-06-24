package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"prophet-trader/configstore"
	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

type ConfigController struct{}

func NewConfigController() *ConfigController {
	return &ConfigController{}
}

func (cc *ConfigController) broadcastConfig() {
	if services.Hub != nil {
		services.Hub.Broadcast("config", configstore.Get())
	}
}

func (cc *ConfigController) HandleGetConfig(c *gin.Context) {
	c.JSON(200, configstore.Get())
}

// --- Agents ---

func (cc *ConfigController) HandleGetAgents(c *gin.Context) {
	cfg := configstore.Get()
	c.JSON(200, gin.H{"agents": cfg.Agents, "activeId": cfg.ActiveAgentID})
}

func (cc *ConfigController) HandleCreateAgent(c *gin.Context) {
	var agent configstore.Agent
	if err := c.BindJSON(&agent); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	created, err := configstore.AddAgent(agent)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "agent": created})
}

func (cc *ConfigController) HandleUpdateAgent(c *gin.Context) {
	id := c.Param("id")
	var updates map[string]interface{}
	if err := c.BindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	agent, err := configstore.UpdateAgent(id, updates)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "agent": agent})
}

func (cc *ConfigController) HandleDeleteAgent(c *gin.Context) {
	if err := configstore.RemoveAgent(c.Param("id")); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

func (cc *ConfigController) HandleActivateAgent(c *gin.Context) {
	if err := configstore.SetActiveAgent(c.Param("id")); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

// --- Strategies ---

func (cc *ConfigController) HandleGetStrategies(c *gin.Context) {
	cfg := configstore.Get()
	c.JSON(200, gin.H{"strategies": cfg.Strategies})
}

func (cc *ConfigController) HandleCreateStrategy(c *gin.Context) {
	var s configstore.Strategy
	if err := c.BindJSON(&s); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	created, err := configstore.AddStrategy(s)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "strategy": created})
}

func (cc *ConfigController) HandleUpdateStrategy(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.BindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	s, err := configstore.UpdateStrategy(c.Param("id"), updates)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "strategy": s})
}

func (cc *ConfigController) HandleDeleteStrategy(c *gin.Context) {
	if err := configstore.RemoveStrategy(c.Param("id")); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

// --- Models ---

func (cc *ConfigController) HandleGetModels(c *gin.Context) {
	cfg := configstore.Get()
	models := cfg.Models
	provider := c.Query("provider")

	if provider != "" {
		filtered := make([]configstore.Model, 0)
		for _, m := range models {
			if strings.HasPrefix(m.ID, provider+"/") {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}

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

	c.JSON(200, gin.H{
		"models":       models,
		"activeModel":  cfg.ActiveModel,
		"providers":    providerList,
		"allProviders": providerList,
	})
}

func (cc *ConfigController) HandleActivateModel(c *gin.Context) {
	var req struct {
		Model string `json:"model"`
	}
	if err := c.BindJSON(&req); err != nil || req.Model == "" {
		c.JSON(400, gin.H{"error": "model is required"})
		return
	}
	if err := configstore.SetActiveModel(req.Model); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	parts := strings.SplitN(req.Model, "/", 2)
	if len(parts) == 2 {
		provider := parts[0]
		if provider == "google" {
			os.Setenv("LLM_PROVIDER", "gemini")
		} else {
			os.Setenv("LLM_PROVIDER", "anthropic")
		}
		os.Setenv("LLM_MODEL", parts[1])
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

func (cc *ConfigController) HandleRefreshModels(c *gin.Context) {
	models := make([]configstore.Model, 0)
	seen := make(map[string]bool)

	add := func(id, name, desc string) {
		if !seen[id] {
			seen[id] = true
			models = append(models, configstore.Model{ID: id, Name: name, Description: desc})
		}
	}

	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		client := &http.Client{Timeout: 10 * time.Second}
		req, _ := http.NewRequest("GET", "https://api.anthropic.com/v1/models", nil)
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var result struct {
				Data []struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
				} `json:"data"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil {
				for _, m := range result.Data {
					name := m.DisplayName
					if name == "" {
						name = strings.ReplaceAll(m.ID, "-", " ")
					}
					add("anthropic/"+m.ID, name, m.ID)
				}
			}
		}
	}

	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		client := &http.Client{Timeout: 10 * time.Second}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", key)
		if resp, err := client.Get(url); err == nil {
			defer resp.Body.Close()
			var result struct {
				Models []struct {
					Name                     string   `json:"name"`
					DisplayName              string   `json:"displayName"`
					Description              string   `json:"description"`
					SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
				} `json:"models"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil {
				for _, m := range result.Models {
					if !strings.HasPrefix(m.Name, "models/gemini-") {
						continue
					}
					supportsGenerate := false
					for _, method := range m.SupportedGenerationMethods {
						if method == "generateContent" {
							supportsGenerate = true
							break
						}
					}
					if !supportsGenerate {
						continue
					}
					modelID := strings.TrimPrefix(m.Name, "models/")
					name := m.DisplayName
					if name == "" {
						name = modelID
					}
					add("google/"+modelID, name, m.Description)
				}
			}
		}
	}

	if len(models) > 0 {
		configstore.SetModels(models)
		cc.broadcastConfig()
	}
	c.JSON(200, gin.H{"ok": true, "count": len(models)})
}

// --- Heartbeat ---

func (cc *ConfigController) HandleGetHeartbeat(c *gin.Context) {
	cfg := configstore.Get()
	c.JSON(200, cfg.Heartbeat)
}

func (cc *ConfigController) HandleUpdateHeartbeat(c *gin.Context) {
	var phases map[string]int
	if err := c.BindJSON(&phases); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := configstore.UpdateHeartbeat(phases); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

func (cc *ConfigController) HandleGetHeartbeatProfiles(c *gin.Context) {
	c.JSON(200, gin.H{"profiles": configstore.HeartbeatProfiles})
}

func (cc *ConfigController) HandleApplyHeartbeatProfile(c *gin.Context) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := c.BindJSON(&req); err != nil || req.Profile == "" {
		c.JSON(400, gin.H{"error": "profile is required"})
		return
	}
	if err := configstore.ApplyHeartbeatProfile(req.Profile); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "profile": req.Profile})
}

func (cc *ConfigController) HandleGetPhases(c *gin.Context) {
	c.JSON(200, gin.H{"phases": configstore.PhaseTimeRanges})
}

func (cc *ConfigController) HandleUpdatePhases(c *gin.Context) {
	var req struct {
		Phase string `json:"phase"`
		Start *int   `json:"start"`
		End   *int   `json:"end"`
	}
	if err := c.BindJSON(&req); err != nil || req.Phase == "" {
		c.JSON(400, gin.H{"error": "phase is required"})
		return
	}
	if err := configstore.UpdatePhaseTimeRange(req.Phase, req.Start, req.End); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true, "phases": configstore.PhaseTimeRanges})
}

// --- Permissions ---

func (cc *ConfigController) HandleGetPermissions(c *gin.Context) {
	c.JSON(200, configstore.GetPermissions())
}

func (cc *ConfigController) HandleUpdatePermissions(c *gin.Context) {
	var perms map[string]interface{}
	if err := c.BindJSON(&perms); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := configstore.UpdatePermissions(perms); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

// --- Plugins ---

func (cc *ConfigController) HandleGetPlugins(c *gin.Context) {
	cfg := configstore.Get()
	c.JSON(200, cfg.Plugins)
}

func (cc *ConfigController) HandleGetPlugin(c *gin.Context) {
	plugin := configstore.GetPlugin(c.Param("name"))
	if plugin == nil {
		c.JSON(200, gin.H{})
		return
	}
	c.JSON(200, plugin)
}

func (cc *ConfigController) HandleUpdatePlugin(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.BindJSON(&updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := configstore.UpdatePlugin(c.Param("name"), updates); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cc.broadcastConfig()
	c.JSON(200, gin.H{"ok": true})
}

func (cc *ConfigController) HandleTestSlack(c *gin.Context) {
	plugin := configstore.GetPlugin("slack")
	slack, ok := plugin.(configstore.SlackPlugin)
	if !ok || slack.WebhookURL == "" {
		c.JSON(400, gin.H{"error": "No Slack webhook URL configured"})
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"text":    ":robot_face: *Prophet Agent* - Test notification\nSlack integration is working!",
		"channel": slack.Channel,
	})
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(slack.WebhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to send test message: " + err.Error()})
		return
	}
	resp.Body.Close()
	c.JSON(200, gin.H{"ok": true})
}
