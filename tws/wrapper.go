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
}
