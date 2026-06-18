package tws

import (
	"reflect"
	"testing"
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
