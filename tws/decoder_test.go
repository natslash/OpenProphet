package tws

import (
	"testing"

	"github.com/shopspring/decimal"
)

type mockWrapper struct {
	DefaultWrapper
	nextValidId     int64
	managedAccts    string
	errReqId        int
	errCode         int
	errMsg          string
	currentTimeSecs int64
	cdReqId         int64
	cdDetails       ContractDetails
	cdEndReqId      int64

	tpReqId   int64
	tpTick    int
	tpPrice   float64
	tpSize    decimal.Decimal
	tpAttr    TickAttrib

	tsReqId   int64
	tsTick    int
	tsSize    decimal.Decimal

	histReqId int64
	histBars  []HistoricalBar
	histEndStart string
	histEndEnd   string

	histUpdReqId int64
	histUpdBar   HistoricalBar

	portfolioContract    Contract
	portfolioPosition    decimal.Decimal
	portfolioMarketPrice float64
	portfolioMarketValue float64
	portfolioAvgCost     float64
	portfolioUnrealPNL   float64
	portfolioRealPNL     float64
	portfolioAccount     string
	acctDownloadEndAcct  string
}

func (m *mockWrapper) HistoricalData(reqId int64, bar HistoricalBar) {
	m.histReqId = reqId
	m.histBars = append(m.histBars, bar)
}

func (m *mockWrapper) HistoricalDataEnd(reqId int64, startDateStr, endDateStr string) {
	m.histReqId = reqId
	m.histEndStart = startDateStr
	m.histEndEnd = endDateStr
}

func (m *mockWrapper) HistoricalDataUpdate(reqId int64, bar HistoricalBar) {
	m.histUpdReqId = reqId
	m.histUpdBar = bar
}

func (m *mockWrapper) NextValidId(orderId int64) {
	m.nextValidId = orderId
}

func (m *mockWrapper) ManagedAccounts(accountsList string) {
	m.managedAccts = accountsList
}

func (m *mockWrapper) Error(reqId int, code int, msg string) {
	m.errReqId = reqId
	m.errCode = code
	m.errMsg = msg
}

func (m *mockWrapper) CurrentTime(timeInSeconds int64) {
	m.currentTimeSecs = timeInSeconds
}

func (m *mockWrapper) ContractDetails(reqId int64, details ContractDetails) {
	m.cdReqId = reqId
	m.cdDetails = details
}

func (m *mockWrapper) ContractDetailsEnd(reqId int64) {
	m.cdEndReqId = reqId
}

func (m *mockWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr TickAttrib) {
	m.tpReqId = reqId
	m.tpTick = tickType
	m.tpPrice = price
	m.tpSize = size
	m.tpAttr = attr
}

func (m *mockWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	m.tsReqId = reqId
	m.tsTick = tickType
	m.tsSize = size
}

func (m *mockWrapper) AccountSummary(reqId int64, account, tag, value, currency string) {}
func (m *mockWrapper) AccountSummaryEnd(reqId int64) {}
func (m *mockWrapper) Position(account string, contract Contract, position decimal.Decimal, avgCost float64) {}
func (m *mockWrapper) PositionEnd() {}
func (m *mockWrapper) OpenOrder(orderId int64, contract Contract, order Order, orderState OrderState) {}
func (m *mockWrapper) OpenOrderEnd() {}
func (m *mockWrapper) OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64) {}

func (m *mockWrapper) UpdatePortfolio(contract Contract, position decimal.Decimal, marketPrice, marketValue, averageCost, unrealizedPNL, realizedPNL float64, accountName string) {
	m.portfolioContract = contract
	m.portfolioPosition = position
	m.portfolioMarketPrice = marketPrice
	m.portfolioMarketValue = marketValue
	m.portfolioAvgCost = averageCost
	m.portfolioUnrealPNL = unrealizedPNL
	m.portfolioRealPNL = realizedPNL
	m.portfolioAccount = accountName
}

func (m *mockWrapper) AccountDownloadEnd(accountName string) {
	m.acctDownloadEndAcct = accountName
}

func TestDecoder_Decode(t *testing.T) {
	mock := &mockWrapper{}
	decoder := NewDecoder(mock)

	tests := []struct {
		name       string
		fields     []string
		validation func(t *testing.T, m *mockWrapper)
	}{
		{
			name:   "next valid id",
			fields: []string{"9", "1", "100"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.nextValidId != 100 {
					t.Errorf("Expected NextValidId 100, got %d", m.nextValidId)
				}
			},
		},
		{
			name:   "managed accounts",
			fields: []string{"15", "1", "DU123,DU456"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.managedAccts != "DU123,DU456" {
					t.Errorf("Expected ManagedAccounts 'DU123,DU456', got %s", m.managedAccts)
				}
			},
		},
		{
			name:   "error message",
			fields: []string{"4", "2", "50", "2104", "Market data farm connection is OK"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.errReqId != 50 || m.errCode != 2104 || m.errMsg != "Market data farm connection is OK" {
					t.Errorf("Expected error 50/2104/Msg, got %d/%d/%s", m.errReqId, m.errCode, m.errMsg)
				}
			},
		},
		{
			name:   "current time",
			fields: []string{"49", "1", "1680000000"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.currentTimeSecs != 1680000000 {
					t.Errorf("Expected time 1680000000, got %v", m.currentTimeSecs)
				}
			},
		},
		{
			name:   "contract data",
			fields: []string{
				"10", "8", "42", "ESTX50", "OPT", "20260619", "5200.0", "C", "EUREX", "EUR",
				"OESX", "ESTX50", "OESX", "12345", "1.0", "0", "10", "LMT", "EUREX", "0", "0",
				"Euro Stoxx 50", "", "", "", "", "", "", "", "", "", "",
			},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.cdReqId != 42 {
					t.Errorf("Expected CD reqId 42, got %d", m.cdReqId)
				}
				if m.cdDetails.Contract.Symbol != "ESTX50" || m.cdDetails.Contract.ConId != 12345 {
					t.Errorf("Expected CD symbol ESTX50, conId 12345, got %v", m.cdDetails)
				}
			},
		},
		{
			name:   "contract data end",
			fields: []string{"52", "1", "42"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.cdEndReqId != 42 {
					t.Errorf("Expected CD End reqId 42, got %d", m.cdEndReqId)
				}
			},
		},
		{
			name:   "tick price",
			fields: []string{"1", "6", "100", "1", "150.25", "100", "1"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.tpReqId != 100 || m.tpTick != 1 || m.tpPrice != 150.25 {
					t.Errorf("Expected TP 100/1/150.25, got %d/%d/%f", m.tpReqId, m.tpTick, m.tpPrice)
				}
				if m.tpSize.String() != "100" {
					t.Errorf("Expected TP size 100, got %s", m.tpSize.String())
				}
				if !m.tpAttr.CanAutoExecute || m.tpAttr.PastLimit || m.tpAttr.PreOpen {
					t.Errorf("Expected attr CanAutoExecute=true, PastLimit=false, PreOpen=false, got %+v", m.tpAttr)
				}
			},
		},
		{
			name:   "tick size with decimal",
			fields: []string{"2", "1", "100", "5", "123.45"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.tsReqId != 100 || m.tsTick != 5 {
					t.Errorf("Expected TS 100/5, got %d/%d", m.tsReqId, m.tsTick)
				}
				if m.tsSize.String() != "123.45" {
					t.Errorf("Expected decimal size 123.45, got %s", m.tsSize.String())
				}
			},
		},
		{
			name:   "empty payload",
			fields: []string{},
			validation: func(t *testing.T, m *mockWrapper) {
				// No panic, no changes
			},
		},
		{
			name:   "historical data",
			// msgID(17), reqId(101), startDate(start), endDate(end), itemCount(1), date, open, high, low, close, volume, wap, barCount
			fields: []string{"17", "101", "20260601", "20260602", "1", "20260601  00:00:00", "5000", "5010", "4990", "5005", "100", "5002.5", "1000"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.histReqId != 101 {
					t.Errorf("Expected histReqId 101, got %d", m.histReqId)
				}
				if len(m.histBars) != 1 {
					t.Errorf("Expected 1 bar, got %d", len(m.histBars))
					return
				}
				bar := m.histBars[0]
				if bar.Date != "20260601  00:00:00" || bar.Open != 5000 || bar.BarCount != 1000 {
					t.Errorf("Unexpected bar data: %+v", bar)
				}
				if m.histEndStart != "20260601" || m.histEndEnd != "20260602" {
					t.Errorf("Unexpected histEnd: %s, %s", m.histEndStart, m.histEndEnd)
				}
			},
		},
		{
			name:   "historical data update",
			// msgID(90), reqId(102), barCount(5), date, open, close, high, low, wap, volume
			fields: []string{"90", "102", "5", "20260601  10:00:00", "10", "12", "13", "9", "11.5", "50"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.histUpdReqId != 102 {
					t.Errorf("Expected histUpdReqId 102, got %d", m.histUpdReqId)
				}
				bar := m.histUpdBar
				if bar.Date != "20260601  10:00:00" || bar.Open != 10 || bar.Volume.String() != "50" || bar.Close != 12 {
					t.Errorf("Unexpected updated bar data: %+v", bar)
				}
			},
		},
		{
			name:   "historical data end standalone",
			fields: []string{"108", "103", "start", "end"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.histReqId != 103 {
					t.Errorf("Expected histReqId 103, got %d", m.histReqId)
				}
				if m.histEndStart != "start" || m.histEndEnd != "end" {
					t.Errorf("Unexpected histEnd: %s, %s", m.histEndStart, m.histEndEnd)
				}
			},
		},
		{
			name: "portfolio value v8 OESX option",
			// msgID(7), version(8), conId, symbol, secType, expiry, strike, right,
			// multiplier, primaryExch, currency, localSymbol, tradingClass,
			// position, marketPrice, marketValue, averageCost, unrealizedPNL, realizedPNL, accountName
			fields: []string{
				"7", "8",
				"12345", "ESTX50", "OPT", "20260620", "5200.0", "C",
				"10", "EUREX", "EUR", "OESX JUN25 5200 C", "OESX",
				"2", "8.50", "170.0", "83.50", "3.0", "0.0", "DU5894187",
			},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.portfolioContract.ConId != 12345 {
					t.Errorf("Expected conId 12345, got %d", m.portfolioContract.ConId)
				}
				if m.portfolioContract.Symbol != "ESTX50" {
					t.Errorf("Expected symbol ESTX50, got %s", m.portfolioContract.Symbol)
				}
				if m.portfolioContract.SecType != "OPT" {
					t.Errorf("Expected secType OPT, got %s", m.portfolioContract.SecType)
				}
				if m.portfolioContract.Multiplier != "10" {
					t.Errorf("Expected multiplier 10, got %s", m.portfolioContract.Multiplier)
				}
				if m.portfolioContract.TradingClass != "OESX" {
					t.Errorf("Expected tradingClass OESX, got %s", m.portfolioContract.TradingClass)
				}
				if !m.portfolioPosition.Equal(decimal.NewFromInt(2)) {
					t.Errorf("Expected position 2, got %s", m.portfolioPosition)
				}
				if m.portfolioMarketPrice != 8.50 {
					t.Errorf("Expected marketPrice 8.50, got %f", m.portfolioMarketPrice)
				}
				if m.portfolioMarketValue != 170.0 {
					t.Errorf("Expected marketValue 170, got %f", m.portfolioMarketValue)
				}
				if m.portfolioAvgCost != 83.50 {
					t.Errorf("Expected avgCost 83.50, got %f", m.portfolioAvgCost)
				}
				if m.portfolioUnrealPNL != 3.0 {
					t.Errorf("Expected unrealizedPNL 3.0, got %f", m.portfolioUnrealPNL)
				}
				if m.portfolioAccount != "DU5894187" {
					t.Errorf("Expected account DU5894187, got %s", m.portfolioAccount)
				}
			},
		},
		{
			name:   "account download end",
			fields: []string{"54", "1", "DU5894187"},
			validation: func(t *testing.T, m *mockWrapper) {
				if m.acctDownloadEndAcct != "DU5894187" {
					t.Errorf("Expected account DU5894187, got %s", m.acctDownloadEndAcct)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder.SetServerVersion(100) // Simulate server version < 196 for start/end date behavior
			err := decoder.Decode(tt.fields)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			tt.validation(t, mock)
		})
	}
}
