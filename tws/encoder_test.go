package tws

import (
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
)

type mockFieldWriter struct {
	fields []string
}

func (m *mockFieldWriter) SendFields(fields ...string) error {
	m.fields = fields
	return nil
}

func TestEncoder_ReqCurrentTime(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	err := encoder.ReqCurrentTime()
	if err != nil {
		t.Fatalf("ReqCurrentTime failed: %v", err)
	}

	expected := []string{"49", "1"}
	if !reflect.DeepEqual(mockWriter.fields, expected) {
		t.Errorf("Encoder sent %q, want %q", mockWriter.fields, expected)
	}
}

func TestEncoder_ReqContractDetails(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	contract := Contract{
		ConId:        12345,
		Symbol:       "ESTX50",
		SecType:      Option,
		LastTradeDateOrContractMonth: "20260619",
		Strike:       5200.0,
		Right:        "C",
		Multiplier:   "10",
		Exchange:     "EUREX",
		Currency:     "EUR",
		TradingClass: "OESX",
	}

	err := encoder.ReqContractDetails(42, contract)
	if err != nil {
		t.Fatalf("ReqContractDetails failed: %v", err)
	}

	expected := []string{
		"9", "8", "42", "12345", "ESTX50", "OPT", "20260619",
		"5200", "C", "10", "EUREX", "", "EUR", "", "OESX", "0", "", "", "",
	}
	if !reflect.DeepEqual(mockWriter.fields, expected) {
		t.Errorf("Encoder sent %q, want %q", mockWriter.fields, expected)
	}

	// Test zero values
	mockWriter.fields = nil
	contractZero := Contract{Symbol: "ESTX50", SecType: Option, Exchange: "EUREX", Currency: "EUR"}
	_ = encoder.ReqContractDetails(43, contractZero)
	expectedZero := []string{
		"9", "8", "43", "0", "ESTX50", "OPT", "",
		"0.0", "", "", "EUREX", "", "EUR", "", "", "0", "", "", "",
	}
	if !reflect.DeepEqual(mockWriter.fields, expectedZero) {
		t.Errorf("Encoder sent %q, want %q", mockWriter.fields, expectedZero)
	}
}

func TestEncoder_ReqHistoricalData(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	contract := Contract{
		ConId:        12345,
		Symbol:       "ESTX50",
		SecType:      Option,
		LastTradeDateOrContractMonth: "20260619",
		Strike:       5200.0,
		Right:        "C",
		Multiplier:   "10",
		Exchange:     "EUREX",
		Currency:     "EUR",
		TradingClass: "OESX",
	}

	err := encoder.ReqHistoricalData(170, 42, contract, "20260601 00:00:00", "1 D", "1 day", "TRADES", 1, 1, false)
	if err != nil {
		t.Fatalf("ReqHistoricalData failed: %v", err)
	}

	expected := []string{
		"20", // outReqHistoricalData
		"42", // reqId
		"12345", // conId
		"ESTX50", "OPT", "20260619", "5200.0", "C", "10", "EUREX", "", "EUR", "", // contract fields
		"OESX", // tradingClass
		"0", // includeExpired
		"20260601 00:00:00", "1 day", // endDateTime, barSizeSetting
		"1 D", "1", "TRADES", // durationStr, useRTH, whatToShow
		"1", // formatDate
		"0", // keepUpToDate
		"", // chartOptions
	}
	
	if !reflect.DeepEqual(mockWriter.fields, expected) {
		t.Errorf("Encoder sent \n%q, want \n%q", mockWriter.fields, expected)
	}
}

func TestEncoder_PlaceOrder_BAG_ComboLegs(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	contract := Contract{
		Symbol:   "ESTX50",
		SecType:  Bag,
		Exchange: "EUREX",
		Currency: "EUR",
		ComboLegs: []ComboLeg{
			{ConId: 111, Ratio: 1, Action: "SELL", Exchange: "EUREX"},
			{ConId: 222, Ratio: 1, Action: "BUY", Exchange: "EUREX"},
		},
	}
	order := Order{
		Action:        "BUY",
		TotalQuantity: decimal.NewFromInt(1),
		OrderType:     "LMT",
		LmtPrice:      2.0,
		AuxPrice:      UnsetFloat,
		Tif:           "DAY",
		Transmit:      true,
	}

	err := encoder.PlaceOrder(187, 42, contract, order)
	if err != nil {
		t.Fatalf("PlaceOrder BAG failed: %v", err)
	}

	fields := mockWriter.fields

	// Find the combo leg count in the output.
	comboIdx := -1
	for i, f := range fields {
		if f == "2" && i > 10 { // "2" = combo legs count, skip early fields
			// Verify the next fields are the two legs
			if i+8 < len(fields) &&
				fields[i+1] == "111" && fields[i+2] == "1" && fields[i+3] == "SELL" && fields[i+4] == "EUREX" &&
				fields[i+9] == "222" && fields[i+10] == "1" && fields[i+11] == "BUY" && fields[i+12] == "EUREX" {
				comboIdx = i
				break
			}
		}
	}
	if comboIdx < 0 {
		t.Fatalf("combo legs not found in PlaceOrder output: %v", fields)
	}
}

func TestEncoder_ReqHistoricalData_BAG_ComboLegs(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	contract := Contract{
		Symbol:   "ESTX50",
		SecType:  Bag,
		Exchange: "EUREX",
		Currency: "EUR",
		ComboLegs: []ComboLeg{
			{ConId: 111, Ratio: 1, Action: "SELL", Exchange: "EUREX"},
			{ConId: 222, Ratio: 1, Action: "BUY", Exchange: "EUREX"},
		},
	}

	err := encoder.ReqHistoricalData(187, 42, contract, "20260601 00:00:00", "1 D", "1 day", "TRADES", 1, 1, false)
	if err != nil {
		t.Fatalf("ReqHistoricalData BAG failed: %v", err)
	}

	fields := mockWriter.fields
	// Verify combo legs are encoded: count "2", then each leg's conId/ratio/action/exchange
	found := false
	for i, f := range fields {
		if f == "2" && i > 5 && i+8 < len(fields) &&
			fields[i+1] == "111" && fields[i+2] == "1" && fields[i+3] == "SELL" && fields[i+4] == "EUREX" &&
			fields[i+5] == "222" && fields[i+6] == "1" && fields[i+7] == "BUY" && fields[i+8] == "EUREX" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("combo legs not found in ReqHistoricalData output: %v", fields)
	}
}

func TestEncoder_ReqSecDefOptParams(t *testing.T) {
	mockWriter := &mockFieldWriter{}
	encoder := NewEncoder(mockWriter)

	err := encoder.ReqSecDefOptParams(42, "ESTX50", "", "IND", 11004968)
	if err != nil {
		t.Fatalf("ReqSecDefOptParams failed: %v", err)
	}

	expected := []string{"78", "42", "ESTX50", "", "IND", "11004968"}
	if !reflect.DeepEqual(mockWriter.fields, expected) {
		t.Errorf("Encoder sent %q, want %q", mockWriter.fields, expected)
	}
}
