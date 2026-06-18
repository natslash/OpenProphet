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
	
	AccountSummary(reqId int64, account, tag, value, currency string)
	AccountSummaryEnd(reqId int64)
	Position(account string, contract Contract, position decimal.Decimal, avgCost float64)
	PositionEnd()
	OpenOrder(orderId int64, contract Contract, order Order, orderState OrderState)
	OpenOrderEnd()
	OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64)
}
