package configstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Agent struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	SystemPromptTemplate string            `json:"systemPromptTemplate"`
	CustomSystemPrompt   string            `json:"customSystemPrompt"`
	StrategyID           *string           `json:"strategyId"`
	Model                string            `json:"model"`
	HeartbeatOverrides   map[string]int    `json:"heartbeatOverrides"`
	CreatedAt            string            `json:"createdAt"`
	UpdatedAt            string            `json:"updatedAt,omitempty"`
}

type Strategy struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	RulesFile   *string `json:"rulesFile"`
	CustomRules *string `json:"customRules"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
}

type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SlackNotifyOn struct {
	TradeExecuted  bool `json:"tradeExecuted"`
	AgentStartStop bool `json:"agentStartStop"`
	Errors         bool `json:"errors"`
	DailySummary   bool `json:"dailySummary"`
	PositionOpened bool `json:"positionOpened"`
	PositionClosed bool `json:"positionClosed"`
	Heartbeat      bool `json:"heartbeat"`
}

type SlackPlugin struct {
	Enabled    bool          `json:"enabled"`
	WebhookURL string        `json:"webhookUrl"`
	Channel    string        `json:"channel"`
	NotifyOn   SlackNotifyOn `json:"notifyOn"`
}

type Plugins struct {
	Slack SlackPlugin `json:"slack"`
}

type Permissions struct {
	AllowLiveTrading     bool     `json:"allowLiveTrading"`
	MaxPositionPct       float64  `json:"maxPositionPct"`
	MaxDeployedPct       float64  `json:"maxDeployedPct"`
	MaxDailyLoss         float64  `json:"maxDailyLoss"`
	MaxOpenPositions     int      `json:"maxOpenPositions"`
	MaxOrderValue        float64  `json:"maxOrderValue"`
	AllowedTools         []string `json:"allowedTools"`
	BlockedTools         []string `json:"blockedTools"`
	AllowOptions         bool     `json:"allowOptions"`
	AllowStocks          bool     `json:"allowStocks"`
	Allow0DTE            bool     `json:"allow0DTE"`
	RequireConfirmation  bool     `json:"requireConfirmation"`
	MaxToolRoundsPerBeat int      `json:"maxToolRoundsPerBeat"`
}

type PhaseTimeRange struct {
	Label string `json:"label"`
	Start *int   `json:"start"`
	End   *int   `json:"end"`
}

type HeartbeatProfile struct {
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Phases      map[string]int `json:"phases"`
}

type Config struct {
	SchemaVersion int            `json:"schemaVersion"`
	ActiveAgentID string         `json:"activeAgentId"`
	ActiveModel   string         `json:"activeModel"`
	Heartbeat     map[string]int `json:"heartbeat"`
	Permissions   Permissions    `json:"permissions"`
	Plugins       Plugins        `json:"plugins"`
	Agents        []Agent        `json:"agents"`
	Strategies    []Strategy     `json:"strategies"`
	Models        []Model        `json:"models"`
}

var DefaultHeartbeat = map[string]int{
	"pre_market":   900,
	"market_open":  120,
	"midday":       600,
	"market_close": 120,
	"after_hours":  1800,
	"closed":       3600,
}

var DefaultPermissions = Permissions{
	AllowLiveTrading:     true,
	MaxPositionPct:       15,
	MaxDeployedPct:       80,
	MaxDailyLoss:         5,
	MaxOpenPositions:     10,
	MaxOrderValue:        0,
	AllowedTools:         []string{},
	BlockedTools:         []string{},
	AllowOptions:         true,
	AllowStocks:          true,
	Allow0DTE:            false,
	RequireConfirmation:  false,
	MaxToolRoundsPerBeat: 25,
}

var DefaultPlugins = Plugins{
	Slack: SlackPlugin{
		Enabled:    false,
		WebhookURL: "",
		Channel:    "",
		NotifyOn: SlackNotifyOn{
			TradeExecuted:  true,
			AgentStartStop: true,
			Errors:         true,
			DailySummary:   true,
			PositionOpened: true,
			PositionClosed: true,
			Heartbeat:      false,
		},
	},
}

var HeartbeatProfiles = map[string]HeartbeatProfile{
	"active": {
		Label:       "Active Trading",
		Description: "High-frequency monitoring during market hours",
		Phases:      map[string]int{"pre_market": 300, "market_open": 60, "midday": 300, "market_close": 60, "after_hours": 600, "closed": 1800},
	},
	"passive": {
		Label:       "Passive Monitoring",
		Description: "Low-frequency check-ins, hands-off approach",
		Phases:      map[string]int{"pre_market": 1800, "market_open": 600, "midday": 900, "market_close": 600, "after_hours": 3600, "closed": 7200},
	},
	"long_horizon": {
		Label:       "Long Horizon",
		Description: "Weekly/monthly style check-ins for position management",
		Phases:      map[string]int{"pre_market": 7200, "market_open": 3600, "midday": 3600, "market_close": 3600, "after_hours": 7200, "closed": 14400},
	},
	"earnings_season": {
		Label:       "Earnings Season",
		Description: "Heightened vigilance during earnings periods",
		Phases:      map[string]int{"pre_market": 180, "market_open": 30, "midday": 120, "market_close": 30, "after_hours": 300, "closed": 1800},
	},
	"overnight": {
		Label:       "Overnight Hold",
		Description: "Set and forget with minimal overnight checks",
		Phases:      map[string]int{"pre_market": 900, "market_open": 120, "midday": 300, "market_close": 120, "after_hours": 7200, "closed": 10800},
	},
	"scalp": {
		Label:       "Scalp Mode",
		Description: "Rapid-fire execution for day trading",
		Phases:      map[string]int{"pre_market": 60, "market_open": 15, "midday": 30, "market_close": 15, "after_hours": 120, "closed": 600},
	},
}

var PhaseTimeRanges = map[string]PhaseTimeRange{
	"pre_market":   {Label: "Pre-Market", Start: intPtr(240), End: intPtr(570)},
	"market_open":  {Label: "Market Open", Start: intPtr(570), End: intPtr(630)},
	"midday":       {Label: "Midday", Start: intPtr(630), End: intPtr(900)},
	"market_close": {Label: "Market Close", Start: intPtr(900), End: intPtr(960)},
	"after_hours":  {Label: "After Hours", Start: intPtr(960), End: intPtr(1200)},
	"closed":       {Label: "Markets Closed", Start: nil, End: nil},
}

func intPtr(v int) *int { return &v }

var (
	store *configStore
	once  sync.Once
)

type configStore struct {
	mu     sync.RWMutex
	config *Config
	path   string
}

func Load(path string) error {
	var loadErr error
	once.Do(func() {
		store = &configStore{path: path}
		loadErr = store.load()
	})
	return loadErr
}

func (s *configStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.config = createDefaultConfig()
			return s.saveLocked()
		}
		return err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		s.config = createDefaultConfig()
		return s.saveLocked()
	}

	cfg := createDefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		s.config = createDefaultConfig()
		return s.saveLocked()
	}

	// v2→v3 migration: if old schema has sandboxes, extract config from first sandbox
	if cfg.SchemaVersion != 3 {
		if sandboxes, ok := raw["sandboxes"].(map[string]interface{}); ok {
			for _, sbx := range sandboxes {
				if m, ok := sbx.(map[string]interface{}); ok {
					if agent, ok := m["agent"].(map[string]interface{}); ok {
						if id, ok := agent["activeAgentId"].(string); ok {
							cfg.ActiveAgentID = id
						}
						if model, ok := agent["model"].(string); ok {
							cfg.ActiveModel = model
						}
					}
				}
				break
			}
		}
		cfg.SchemaVersion = 3
	}

	if cfg.Heartbeat == nil {
		cfg.Heartbeat = copyMap(DefaultHeartbeat)
	}
	if cfg.Permissions.AllowedTools == nil {
		cfg.Permissions.AllowedTools = []string{}
	}
	if cfg.Permissions.BlockedTools == nil {
		cfg.Permissions.BlockedTools = []string{}
	}

	s.config = cfg
	return s.saveLocked()
}

func (s *configStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func save() error {
	return store.saveLocked()
}

// Get returns a copy of the current config.
func Get() Config {
	if store == nil {
		return Config{}
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	cfg := *store.config
	cfg.Agents = make([]Agent, len(store.config.Agents))
	copy(cfg.Agents, store.config.Agents)
	cfg.Strategies = make([]Strategy, len(store.config.Strategies))
	copy(cfg.Strategies, store.config.Strategies)
	cfg.Models = make([]Model, len(store.config.Models))
	copy(cfg.Models, store.config.Models)
	cfg.Heartbeat = copyMap(store.config.Heartbeat)
	return cfg
}

// --- Agents ---

func AddAgent(a Agent) (Agent, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	a.ID = shortID()
	if a.Name == "" {
		a.Name = "New Agent"
	}
	if a.SystemPromptTemplate == "" {
		a.SystemPromptTemplate = "custom"
	}
	if a.Model == "" {
		a.Model = store.config.ActiveModel
	}
	if a.HeartbeatOverrides == nil {
		a.HeartbeatOverrides = map[string]int{}
	}
	a.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.config.Agents = append(store.config.Agents, a)
	return a, save()
}

func UpdateAgent(id string, updates map[string]interface{}) (Agent, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	idx := -1
	for i, a := range store.config.Agents {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return Agent{}, fmt.Errorf("agent not found")
	}

	// Marshal the existing agent, merge updates, unmarshal back
	existing, _ := json.Marshal(store.config.Agents[idx])
	var merged map[string]interface{}
	json.Unmarshal(existing, &merged)
	for k, v := range updates {
		merged[k] = v
	}
	merged["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, _ := json.Marshal(merged)
	var updated Agent
	json.Unmarshal(data, &updated)
	store.config.Agents[idx] = updated

	if store.config.ActiveAgentID == id {
		if m, ok := updates["model"].(string); ok && m != "" {
			store.config.ActiveModel = m
		}
	}

	return updated, save()
}

func RemoveAgent(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if id == "default" {
		return fmt.Errorf("cannot remove default agent")
	}

	agents := make([]Agent, 0, len(store.config.Agents))
	for _, a := range store.config.Agents {
		if a.ID != id {
			agents = append(agents, a)
		}
	}
	store.config.Agents = agents

	if store.config.ActiveAgentID == id {
		store.config.ActiveAgentID = "default"
	}
	return save()
}

func SetActiveAgent(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	var found *Agent
	for i := range store.config.Agents {
		if store.config.Agents[i].ID == id {
			found = &store.config.Agents[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("agent not found")
	}
	store.config.ActiveAgentID = id
	if found.Model != "" {
		store.config.ActiveModel = found.Model
	}
	return save()
}

func GetActiveAgent() *Agent {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, a := range store.config.Agents {
		if a.ID == store.config.ActiveAgentID {
			cp := a
			return &cp
		}
	}
	if len(store.config.Agents) > 0 {
		cp := store.config.Agents[0]
		return &cp
	}
	return nil
}

func GetAgentByID(id string) *Agent {
	if store == nil {
		return nil
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, a := range store.config.Agents {
		if a.ID == id {
			cp := a
			return &cp
		}
	}
	return nil
}

func ActiveAgentName() string {
	a := GetActiveAgent()
	if a != nil {
		return a.Name
	}
	return ""
}

func AgentName(id string) string {
	a := GetAgentByID(id)
	if a != nil {
		return a.Name
	}
	return id
}

// --- Strategies ---

func AddStrategy(s Strategy) (Strategy, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	s.ID = shortID()
	if s.Name == "" {
		s.Name = "New Strategy"
	}
	s.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.config.Strategies = append(store.config.Strategies, s)
	return s, save()
}

func UpdateStrategy(id string, updates map[string]interface{}) (Strategy, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	idx := -1
	for i, s := range store.config.Strategies {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return Strategy{}, fmt.Errorf("strategy not found")
	}

	existing, _ := json.Marshal(store.config.Strategies[idx])
	var merged map[string]interface{}
	json.Unmarshal(existing, &merged)
	for k, v := range updates {
		merged[k] = v
	}
	merged["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	data, _ := json.Marshal(merged)
	var updated Strategy
	json.Unmarshal(data, &updated)
	store.config.Strategies[idx] = updated
	return updated, save()
}

func RemoveStrategy(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	if id == "default" {
		return fmt.Errorf("cannot remove default strategy")
	}
	strategies := make([]Strategy, 0, len(store.config.Strategies))
	for _, s := range store.config.Strategies {
		if s.ID != id {
			strategies = append(strategies, s)
		}
	}
	store.config.Strategies = strategies
	return save()
}

func GetStrategyByID(id string) *Strategy {
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, s := range store.config.Strategies {
		if s.ID == id {
			cp := s
			return &cp
		}
	}
	return nil
}

// --- Models ---

func SetActiveModel(modelID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.config.ActiveModel = modelID
	return save()
}

func SetModels(models []Model) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.config.Models = models
	return save()
}

// --- Heartbeat ---

func UpdateHeartbeat(phases map[string]int) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for k, v := range phases {
		store.config.Heartbeat[k] = v
	}
	return save()
}

func GetHeartbeatForPhase(phase string) int {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if v, ok := store.config.Heartbeat[phase]; ok {
		return v
	}
	if v, ok := DefaultHeartbeat[phase]; ok {
		return v
	}
	return 600
}

func ApplyHeartbeatProfile(key string) error {
	profile, ok := HeartbeatProfiles[key]
	if !ok {
		return fmt.Errorf("unknown heartbeat profile: %s", key)
	}
	return UpdateHeartbeat(profile.Phases)
}

func UpdatePhaseTimeRange(phase string, start, end *int) error {
	ptr, ok := PhaseTimeRanges[phase]
	if !ok {
		return fmt.Errorf("unknown phase: %s", phase)
	}
	if start != nil {
		ptr.Start = start
	}
	if end != nil {
		ptr.End = end
	}
	PhaseTimeRanges[phase] = ptr
	return nil
}

// --- Permissions ---

func UpdatePermissions(perms map[string]interface{}) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	existing, _ := json.Marshal(store.config.Permissions)
	var merged map[string]interface{}
	json.Unmarshal(existing, &merged)
	for k, v := range perms {
		merged[k] = v
	}
	data, _ := json.Marshal(merged)
	json.Unmarshal(data, &store.config.Permissions)
	return save()
}

func GetPermissions() Permissions {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.config.Permissions
}

// --- Plugins ---

func UpdatePlugin(name string, updates map[string]interface{}) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	existing, _ := json.Marshal(store.config.Plugins)
	var pluginsMap map[string]interface{}
	json.Unmarshal(existing, &pluginsMap)

	if pluginsMap[name] == nil {
		pluginsMap[name] = map[string]interface{}{}
	}
	plug := pluginsMap[name].(map[string]interface{})
	for k, v := range updates {
		plug[k] = v
	}
	pluginsMap[name] = plug

	data, _ := json.Marshal(pluginsMap)
	json.Unmarshal(data, &store.config.Plugins)
	return save()
}

func GetPlugin(name string) interface{} {
	store.mu.RLock()
	defer store.mu.RUnlock()
	switch name {
	case "slack":
		return store.config.Plugins.Slack
	}
	return nil
}

// --- Helpers ---

func createDefaultConfig() *Config {
	defaultStrategyID := "default"
	return &Config{
		SchemaVersion: 3,
		ActiveAgentID: "default",
		ActiveModel:   "anthropic/claude-sonnet-4-6",
		Heartbeat:     copyMap(DefaultHeartbeat),
		Permissions:   DefaultPermissions,
		Plugins:       DefaultPlugins,
		Agents: []Agent{
			{
				ID: "default", Name: "Prophet",
				Description:          "Aggressive discretionary options trader with scalping overlay",
				SystemPromptTemplate: "default",
				StrategyID:           &defaultStrategyID,
				Model:                "anthropic/claude-sonnet-4-6",
				HeartbeatOverrides:   map[string]int{},
				CreatedAt:            time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
		Strategies: []Strategy{
			{
				ID: "default", Name: "Aggressive Options",
				Description: "Multi-timeframe options with scalping overlay",
				RulesFile:   strPtr("TRADING_RULES.md"),
				CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			},
		},
		Models: defaultModels(),
	}
}

func defaultModels() []Model {
	return []Model{
		{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Description: "Best speed + intelligence, $3/$15 per MTok"},
		{ID: "anthropic/claude-opus-4-8", Name: "Claude Opus 4.8", Description: "Next generation Opus, $15/$75 per MTok"},
		{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", Description: "Google Advanced Pro Model"},
		{ID: "anthropic/claude-opus-4-6", Name: "Claude Opus 4.6", Description: "Most intelligent, best for agents, $5/$25 per MTok"},
		{ID: "anthropic/claude-haiku-4-5", Name: "Claude Haiku 4.5", Description: "Fastest, near-frontier, $1/$5 per MTok"},
	}
}

func shortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func strPtr(s string) *string { return &s }

func copyMap(m map[string]int) map[string]int {
	cp := make(map[string]int, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
