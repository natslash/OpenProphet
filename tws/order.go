package tws

import "github.com/shopspring/decimal"

// Order represents an IBKR TWS Order.
type Order struct {
	OrderId       int64
	ClientId      int
	PermId        int64
	Action        string // "BUY", "SELL"
	TotalQuantity decimal.Decimal
	OrderType     string // "MKT", "LMT", "STP"
	LmtPrice      float64
	AuxPrice      float64
	Tif           string // "DAY", "GTC"
	OcaGroup      string
	Account       string
	OpenClose     string // "O", "C"
	Origin        int
	OrderRef      string
	Transmit      bool
	ParentId      int64
	BlockOrder    bool
	SweepToFill   bool
	DisplaySize   int
	TriggerMethod int
	OutsideRth    bool
	Hidden        bool
}

// OrderState represents the current state of an order.
type OrderState struct {
	Status                  string
	InitMarginBefore        string
	MaintMarginBefore       string
	EquityWithLoanBefore    string
	InitMarginChange        string
	MaintMarginChange       string
	EquityWithLoanChange    string
	InitMarginAfter         string
	MaintMarginAfter        string
	EquityWithLoanAfter     string
	Commission              float64
	MinCommission           float64
	MaxCommission           float64
	CommissionCurrency      string
	WarningText             string
	CompletedTime           string
	CompletedStatus         string
}
