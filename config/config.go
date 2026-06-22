package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	GeminiAPIKey      string
	DatabasePath      string
	ServerPort        string
	EnableLogging     bool
	LogLevel          string
	DataRetentionDays int

	// Broker is now IBKR-only.
	IBKRHost     string
	IBKRPort     int // paper = 4002 (the only permitted target until Phase 6)
	IBKRClientID int

	// TradingEnabled is the master order kill-switch. When false (the default),
	// the trading service runs in dry-run mode: order intent is logged but no
	// order is placed/cancelled. Must be explicitly set true (Phase 4.3e).
	TradingEnabled bool

	// AdminToken authorises autonomous-beat trade intents. The /authorize
	// endpoint requires "Authorization: Bearer <AdminToken>"; the Node agent
	// must NOT have this value. If empty, authorisation fails closed (no intent
	// can be executed) — human-only by construction.
	AdminToken string

	// Autonomous beat (Phase 4.3e). Disabled by default.
	BeatEnabled            bool
	BeatSymbol             string // configured target contract, e.g. "ESTX50:20260619:C:6325"
	BeatIntervalSecs       int
	BeatMaxDailyExecutions int
	BeatForceSignal        bool // testing aid: force a buy signal every tick
	
	// Review Portfolio Polling
	LLMPollingEnabled      bool
	LLMPollingIntervalSecs int
	
	LLMProvider string
	LLMModel    string

	// Intent Guard
	RequireDoubleConfirm    bool
	AllowLivePort           bool
	IntentTTLSeconds        int
	MaxPriceSlippagePercent float64
}

var AppConfig *Config

func Load() error {
	// Load .env and .env.backend files if they exist (don't override existing env vars)
	_ = godotenv.Load(".env", ".env.backend")

	AppConfig = &Config{
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		DatabasePath:      getEnvOrDefault("DATABASE_PATH", "./data/prophet_trader.db"),
		ServerPort:        getEnvOrDefault("PORT", getEnvOrDefault("SERVER_PORT", "4534")),
		EnableLogging:     getEnvOrDefault("ENABLE_LOGGING", "true") == "true",
		LogLevel:          getEnvOrDefault("LOG_LEVEL", "info"),
		DataRetentionDays: 90,

		IBKRHost:       getEnvOrDefault("IBKR_HOST", "127.0.0.1"),
		IBKRPort:       getEnvAsInt("IBKR_PORT", 4002),
		IBKRClientID:   getEnvAsInt("IBKR_CLIENT_ID", 1),
		TradingEnabled: getEnvOrDefault("TRADING_ENABLED", "false") == "true",

		AdminToken:             os.Getenv("ADMIN_TOKEN"),
		BeatEnabled:            getEnvOrDefault("BEAT_ENABLED", "false") == "true",
		BeatSymbol:             getEnvOrDefault("BEAT_SYMBOL", "ESTX50:20260619:C:6325"),
		BeatIntervalSecs:       getEnvAsInt("BEAT_INTERVAL_SECS", 300),
		BeatMaxDailyExecutions: getEnvAsInt("BEAT_MAX_DAILY_EXECUTIONS", 3),
		BeatForceSignal:        getEnvOrDefault("BEAT_FORCE_SIGNAL", "false") == "true",
		
		LLMPollingEnabled:      getEnvOrDefault("LLM_POLLING_ENABLED", "false") == "true",
		LLMPollingIntervalSecs: getEnvAsInt("LLM_POLLING_INTERVAL_SECS", 3600),
		
		LLMProvider:            getEnvOrDefault("LLM_PROVIDER", "anthropic"),
		LLMModel:               getEnvOrDefault("LLM_MODEL", ""),

		RequireDoubleConfirm:    getEnvOrDefault("REQUIRE_DOUBLE_CONFIRM", "true") == "true",
		AllowLivePort:           getEnvOrDefault("ALLOW_LIVE_PORT", "false") == "true",
		IntentTTLSeconds:        getEnvAsInt("INTENT_TTL_SECS", 300),
		MaxPriceSlippagePercent: getEnvAsFloat("MAX_PRICE_SLIPPAGE_PERCENT", 0.5),
	}

	log.Printf("[CONFIG] AdminToken configured: %v\n", AppConfig.AdminToken != "")

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
