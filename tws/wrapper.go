package tws

import "github.com/shopspring/decimal"

// TickAttrib represents attributes associated with a tick.
type TickAttrib struct {
	CanAutoExecute bool
	PastLimit      bool
	PreOpen        bool
}

type TickPriceMsg struct {
	TickType int
	Price    float64
	Size     decimal.Decimal
	Attr     TickAttrib
}

type TickSizeMsg struct {
	TickType int
	Size     decimal.Decimal
}

// MarketDataTypeMsg reports the data tier TWS is serving for a reqId:
// 1=live, 2=frozen, 3=delayed, 4=delayed-frozen. Sent right after reqMktData.
type MarketDataTypeMsg struct {
	Type int
}

type AccountSummaryMsg struct {
	ReqId    int64
	Account  string
	Tag      string
	Value    string
	Currency string
}

type AccountSummaryEndMsg struct {
	ReqId int64
}

type PositionMsg struct {
	Account  string
	Contract Contract
	Position decimal.Decimal
	AvgCost  float64
}

type PositionEndMsg struct {}

type UpdatePortfolioMsg struct {
	Contract      Contract
	Position      decimal.Decimal
	MarketPrice   float64
	MarketValue   float64
	AverageCost   float64
	UnrealizedPNL float64
	RealizedPNL   float64
	AccountName   string
}

type AccountDownloadEndMsg struct {
	AccountName string
}

type OpenOrderMsg struct {
	OrderId int64
	Contract Contract
	Order Order
	OrderState OrderState
}

type OpenOrderEndMsg struct {}

type OrderStatusMsg struct {
	OrderId int64
	Status string
	Filled decimal.Decimal
	Remaining decimal.Decimal
	AvgFillPrice float64
	PermId int64
	ParentId int64
	LastFillPrice float64
	ClientId int
	WhyHeld string
	MktCapPrice float64
}

type HistoricalBar struct {
	Date     string
	Open     float64
	High     float64
	Low      float64
	Close    float64
	Volume   decimal.Decimal
	BarCount int
	WAP      decimal.Decimal
}

type HistoricalDataMsg struct {
	ReqId int64
	Bar   HistoricalBar
}

type HistoricalDataEndMsg struct {
	ReqId   int64
	Start   string
	End     string
	ExtData any
}

type HistoricalDataUpdateMsg struct {
	ReqId int64
	Bar   HistoricalBar
}

type SecDefOptParamsMsg struct {
	Exchange        string
	UnderlyingConId int64
	TradingClass    string
	Multiplier      string
	Expirations     []string
	Strikes         []float64
}

type TickOptionComputationMsg struct {
	TickType   int
	TickAttrib int
	ImpliedVol float64
	Delta      float64
	OptPrice   float64
	PvDividend float64
	Gamma      float64
	Vega       float64
	Theta      float64
	UndPrice   float64
}

// Wrapper is the callback interface for receiving decoded messages from TWS.
// It represents the Go equivalent of the Java EWrapper interface.
type Wrapper interface {
	NextValidId(orderId int64)
	ManagedAccounts(accountsList string)
	Error(reqId int, code int, msg string)
	CurrentTime(timeInSeconds int64)
	ContractDetails(reqId int64, details ContractDetails)
	ContractDetailsEnd(reqId int64)
	TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr TickAttrib)
	TickSize(reqId int64, tickType int, size decimal.Decimal)
	MarketDataType(reqId int64, marketDataType int)

	AccountSummary(reqId int64, account, tag, value, currency string)
	AccountSummaryEnd(reqId int64)
	Position(account string, contract Contract, position decimal.Decimal, avgCost float64)
	PositionEnd()
	UpdatePortfolio(contract Contract, position decimal.Decimal, marketPrice, marketValue, averageCost, unrealizedPNL, realizedPNL float64, accountName string)
	AccountDownloadEnd(accountName string)
	OpenOrder(orderId int64, contract Contract, order Order, orderState OrderState)
	OpenOrderEnd()
	OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64)
	HistoricalData(reqId int64, bar HistoricalBar)
	HistoricalDataEnd(reqId int64, startDateStr, endDateStr string)
	HistoricalDataUpdate(reqId int64, bar HistoricalBar)
	SecurityDefinitionOptionParameter(reqId int64, exchange string, underlyingConId int64, tradingClass, multiplier string, expirations []string, strikes []float64)
	SecurityDefinitionOptionParameterEnd(reqId int64)
	TickOptionComputation(reqId int64, tickType int, tickAttrib int, impliedVol, delta, optPrice, pvDividend, gamma, vega, theta, undPrice float64)
}
