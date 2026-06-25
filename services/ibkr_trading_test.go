package services

import (
	"testing"

	"prophet-trader/tws"
)

// TestLegContractMatches guards the combo-leg inversion fix: a leg must only be
// accepted when the resolved contract matches the requested strike and right.
func TestLegContractMatches(t *testing.T) {
	opt := func(strike float64, right string, conId int64) tws.Contract {
		return tws.Contract{SecType: tws.Option, Strike: strike, Right: right, ConId: conId}
	}
	tests := []struct {
		name string
		want tws.Contract
		got  tws.Contract
		ok   bool
	}{
		{"exact match P4900", opt(4900, "P", 0), opt(4900, "P", 801161443), true},
		{"right case-insensitive", opt(4800, "p", 0), opt(4800, "P", 801161446), true},
		{"wrong strike (the inversion)", opt(4900, "P", 0), opt(4800, "P", 801161446), false},
		{"wrong right", opt(4900, "P", 0), opt(4900, "C", 999), false},
		{"non-option always matches", tws.Contract{SecType: tws.Stock, Symbol: "AAPL"}, tws.Contract{SecType: tws.Stock, Symbol: "AAPL"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legContractMatches(tt.want, tt.got); got != tt.ok {
				t.Errorf("legContractMatches = %v, want %v", got, tt.ok)
			}
		})
	}
}

// TestUnderlyingSymbol verifies the order guard maps an instrument symbol to the
// underlying whose spot must be verified before trading.
func TestUnderlyingSymbol(t *testing.T) {
	cases := map[string]string{
		"ESTX50:20260821:P:4900": "ESTX50",
		"ESTX50":                 "ESTX50",
		"AAPL":                   "AAPL",
	}
	for in, want := range cases {
		if got := underlyingSymbol(in); got != want {
			t.Errorf("underlyingSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeOrderType(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "market", want: "MKT"},
		{in: "MKT", want: "MKT"},
		{in: "limit", want: "LMT"},
		{in: "LMT", want: "LMT"},
		{in: "  Limit ", want: "LMT"}, // trimmed + case-insensitive
		{in: "stop", want: "STP"},
		{in: "stop_limit", want: "STP LMT"},
		{in: "stop limit", want: "STP LMT"},
		{in: "", wantErr: true},   // guardrail: never default to a market order
		{in: "   ", wantErr: true},
		{in: "bogus", wantErr: true},
	}
	for _, tc := range tests {
		got, err := normalizeOrderType(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeOrderType(%q): expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeOrderType(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeOrderType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
