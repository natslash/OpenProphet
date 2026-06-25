package interfaces

import (
	"context"
	"time"
)

// TradingService defines the interface for executing trades
type TradingService interface {
	PlaceOrder(ctx context.Context, order *Order) (*OrderResult, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOrder(ctx context.Context, orderID string) (*Order, error)
	ListOrders(ctx context.Context, status string) ([]*Order, error)
	GetPositions(ctx context.Context) ([]*Position, error)
	GetAccount(ctx context.Context) (*Account, error)

	// Options trading methods
	PlaceOptionsOrder(ctx context.Context, order *OptionsOrder) (*OrderResult, error)
	PlaceComboOrder(ctx context.Context, order *ComboOrder) (*OrderResult, error)
	GetOptionsChain(ctx context.Context, underlying string, expiration time.Time) ([]*OptionContract, error)
	GetOptionsQuote(ctx context.Context, symbol string) (*OptionsQuote, error)
	GetOptionsPosition(ctx context.Context, symbol string) (*OptionsPosition, error)
	ListOptionsPositions(ctx context.Context) ([]*OptionsPosition, error)
}

// DataService defines the interface for market data operations
type DataService interface {
	GetHistoricalBars(ctx context.Context, symbol string, start, end time.Time, timeframe string) ([]*Bar, error)
	GetLatestBar(ctx context.Context, symbol string) (*Bar, error)
	GetLatestQuote(ctx context.Context, symbol string) (*Quote, error)
	GetLatestTrade(ctx context.Context, symbol string) (*Trade, error)
	StreamBars(ctx context.Context, symbols []string) (<-chan *Bar, error)
}

// StorageService defines the interface for local data persistence
type StorageService interface {
	SaveBars(bars []*Bar) error
	GetBars(symbol string, start, end time.Time) ([]*Bar, error)
	SaveOrder(order *Order) error
	GetOrder(orderID string) (*Order, error)
	GetOrders(status string) ([]*Order, error)
	CleanupOldData(before time.Time) error
}

// StrategyExecutor defines the interface for strategy execution
// This will be useful for AI personas and quant strategies later
type StrategyExecutor interface {
	Initialize(config map[string]interface{}) error
	ShouldBuy(ctx context.Context, symbol string, data *MarketData) (bool, *OrderRequest)
	ShouldSell(ctx context.Context, symbol string, data *MarketData) (bool, *OrderRequest)
	OnOrderFilled(order *Order)
	OnMarketData(data *MarketData)
	GetName() string
}

// Common data structures used across interfaces
type Order struct {
	ID            string
	Symbol        string
	Qty           float64
	Side          string // "buy" or "sell"
	Type          string // "market", "limit", etc.
	TimeInForce   string // "day", "gtc", etc.
	LimitPrice    *float64
	StopPrice     *float64 // Stop trigger for entry orders (e.g., stop or stop_limit)
	TakeProfitPrice *float64 // Limit price for the attached take-profit child order
	StopLossPrice   *float64 // Stop trigger price for the attached stop-loss child order
	Status        string
	FilledQty     float64
	FilledAvgPrice *float64
	SubmittedAt   time.Time
	FilledAt      *time.Time
	CanceledAt    *time.Time
}

type OrderRequest struct {
	Symbol      string
	Qty         float64
	Side        string
	Type        string
	TimeInForce string
	LimitPrice  *float64
	StopPrice   *float64
}

// OrderResult represents the result of a placed order
type OrderResult struct {
	OrderID           string
	TakeProfitOrderID string
	StopLossOrderID   string
	Status            string
	Message           string
}

type Position struct {
	Symbol           string  `json:"symbol"`
	Qty              float64 `json:"qty"`
	AvgEntryPrice    float64 `json:"avgEntryPrice"`
	MarketValue      float64 `json:"marketValue"`
	CostBasis        float64 `json:"costBasis"`
	UnrealizedPL     float64 `json:"unrealizedPL"`
	UnrealizedPLPC   float64 `json:"unrealizedPLPC"`
	CurrentPrice     float64 `json:"currentPrice"`
	Side             string  `json:"side"`
}

type Account struct {
	ID               string  `json:"id"`
	Cash             float64 `json:"cash"`
	PortfolioValue   float64 `json:"portfolioValue"`
	BuyingPower      float64 `json:"buyingPower"`
	DayTradeCount    int     `json:"dayTradeCount"`
	PatternDayTrader bool    `json:"patternDayTrader"`
}

type Bar struct {
	Symbol    string
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
	VWAP      float64
}

// Market data tiers reported by IBKR's marketDataType message.
const (
	MarketDataLive          = 1 // real-time
	MarketDataFrozen        = 2 // last available (end-of-day) snapshot — stale
	MarketDataDelayed       = 3 // ~15-min delayed
	MarketDataDelayedFrozen = 4 // delayed last-available snapshot
)

type Quote struct {
	Symbol    string
	BidPrice  float64
	BidSize   int64
	AskPrice  float64
	AskSize   int64
	Timestamp time.Time
	// MarketDataType is the IBKR tier serving this quote (0=unknown, 1=live,
	// 2=frozen, 3=delayed, 4=delayed-frozen). Lets callers reject stale data.
	MarketDataType int
}

// Tradeability reports whether a quote is safe to act on given the maximum
// acceptable age. Frozen (end-of-day) and missing/zero quotes are never
// tradeable; delayed quotes are allowed within maxAge but flagged with a
// warning. An empty warning with ok=true means a clean live quote.
func (q *Quote) Tradeability(now time.Time, maxAge time.Duration) (ok bool, warn string) {
	if q == nil || (q.BidPrice <= 0 && q.AskPrice <= 0) {
		return false, "no quote available"
	}
	if q.MarketDataType == MarketDataFrozen {
		return false, "frozen (end-of-day) market data — not tradeable"
	}
	if age := now.Sub(q.Timestamp); age > maxAge {
		return false, "quote is stale (age " + age.Round(time.Second).String() + " exceeds limit)"
	}
	if q.MarketDataType == MarketDataDelayed || q.MarketDataType == MarketDataDelayedFrozen {
		return true, "delayed market data (~15min) — sizing with caution"
	}
	return true, ""
}

// FreshnessLabel renders a compact human/LLM-readable freshness tag, e.g.
// "LIVE, age 2s" or "DELAYED, age 1s".
func (q *Quote) FreshnessLabel(now time.Time) string {
	tier := "UNKNOWN"
	switch q.MarketDataType {
	case MarketDataLive:
		tier = "LIVE"
	case MarketDataFrozen:
		tier = "FROZEN"
	case MarketDataDelayed:
		tier = "DELAYED"
	case MarketDataDelayedFrozen:
		tier = "DELAYED-FROZEN"
	}
	return tier + ", age " + now.Sub(q.Timestamp).Round(time.Second).String()
}

type Trade struct {
	Symbol    string
	Price     float64
	Size      int64
	Timestamp time.Time
}

type MarketData struct {
	Symbol       string
	CurrentBar   *Bar
	RecentBars   []*Bar
	LatestQuote  *Quote
	LatestTrade  *Trade
	Indicators   map[string]float64 // For calculated indicators
}

type ComboOrder struct {
	Legs        []ComboLeg
	Action      string   // "BUY" or "SELL" (net action)
	Qty         float64
	OrderType   string   // "LMT", "MKT"
	LimitPrice  *float64 // net debit (positive) or credit (negative)
	TimeInForce string
}

type ComboLeg struct {
	Symbol string // e.g. "ESTX50:20260821:P:5450"
	Action string // "BUY" or "SELL"
	Ratio  int
}

// Options trading structures
type OptionsOrder struct {
	Symbol        string  // Options symbol in OCC format (e.g., TSLA251219C00400000)
	Underlying    string  // Underlying stock symbol
	Qty           float64
	Side          string // "buy" or "sell"
	PositionIntent string // "buy_to_open", "buy_to_close", "sell_to_open", "sell_to_close"
	Type          string // "market", "limit"
	TimeInForce   string // "day", "gtc"
	LimitPrice    *float64
}

type OptionsQuote struct {
	Symbol    string
	BidPrice  float64
	BidSize   int64
	AskPrice  float64
	AskSize   int64
	LastPrice float64
	Volume    int64
	Timestamp time.Time
}

type OptionsPosition struct {
	Symbol        string    `json:"symbol"`
	Underlying    string    `json:"underlying"`
	Qty           float64   `json:"qty"`
	AvgEntryPrice float64   `json:"avgEntryPrice"`
	MarketValue   float64   `json:"marketValue"`
	CostBasis     float64   `json:"costBasis"`
	UnrealizedPL  float64   `json:"unrealizedPL"`
	UnrealizedPLPC float64  `json:"unrealizedPLPC"`
	CurrentPrice  float64   `json:"currentPrice"`
	Side          string    `json:"side"` // "long" or "short"
	Expiration    time.Time `json:"expiration"`
	Strike        float64   `json:"strike"`
	OptionType    string    `json:"optionType"` // "call" or "put"
}