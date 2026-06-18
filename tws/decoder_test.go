package tws

import (
	"testing"

	"github.com/shopspring/decimal"
)

type mockWrapper struct {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decoder.Decode(tt.fields)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}
			tt.validation(t, mock)
		})
	}
}
