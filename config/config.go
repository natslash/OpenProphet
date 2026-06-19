package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AlpacaAPIKey      string
	AlpacaSecretKey   string
	AlpacaBaseURL     string
	AlpacaPaper       bool
	GeminiAPIKey      string
	DatabasePath      string
	ServerPort        string
	EnableLogging     bool
	LogLevel          string
	DataRetentionDays int

	// Broker selection — temporary A/B aid during the IBKR build; end state is
	// IBKR-only (Alpaca deleted at the Phase 5 cutover). Default "alpaca".
	Broker       string // "alpaca" | "ibkr"
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
}

var AppConfig *Config

func Load() error {
	// Load .env file if it exists (don't override existing env vars)
	_ = godotenv.Load()

	AppConfig = &Config{
		AlpacaAPIKey:      os.Getenv("ALPACA_API_KEY"),
		AlpacaSecretKey:   os.Getenv("ALPACA_SECRET_KEY"),
		AlpacaBaseURL:     getEnvOrDefault("ALPACA_BASE_URL", "https://paper-api.alpaca.markets"),
		AlpacaPaper:       getEnvOrDefault("ALPACA_PAPER", "true") == "true",
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		DatabasePath:      getEnvOrDefault("DATABASE_PATH", "./data/prophet_trader.db"),
		ServerPort:        getEnvOrDefault("PORT", getEnvOrDefault("SERVER_PORT", "4534")),
		EnableLogging:     getEnvOrDefault("ENABLE_LOGGING", "true") == "true",
		LogLevel:          getEnvOrDefault("LOG_LEVEL", "info"),
		DataRetentionDays: 90,

		Broker:         getEnvOrDefault("BROKER", "alpaca"),
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
	}

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
