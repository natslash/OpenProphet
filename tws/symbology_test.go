package tws

import "testing"

func TestParseSymbol(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Contract
		wantErr bool
	}{
		{
			name: "us stock",
			in:   "AAPL",
			want: Contract{Symbol: "AAPL", SecType: Stock, Exchange: "SMART", Currency: "USD"},
		},
		{
			name: "oesx call",
			in:   "OESX:20260620:C:5200",
			want: Contract{
				Symbol: "ESTX50", SecType: Option, Exchange: "EUREX", Currency: "EUR",
				LastTradeDateOrContractMonth: "20260620", Strike: 5200, Right: "C",
				Multiplier: "10", TradingClass: "OESX",
			},
		},
		{
			name: "oesx put lowercase right",
			in:   "OESX:20260620:p:4800",
			want: Contract{
				Symbol: "ESTX50", SecType: Option, Exchange: "EUREX", Currency: "EUR",
				LastTradeDateOrContractMonth: "20260620", Strike: 4800, Right: "P",
				Multiplier: "10", TradingClass: "OESX",
			},
		},
		{name: "empty", in: "", wantErr: true},
		{name: "oesx wrong arity", in: "OESX:20260620:C", wantErr: true},
		{name: "oesx bad right", in: "OESX:20260620:X:5200", wantErr: true},
		{name: "oesx bad strike", in: "OESX:20260620:C:abc", wantErr: true},
		{name: "unknown prefix", in: "FOO:bar", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSymbol(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSymbol(%q): expected error, got %+v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSymbol(%q): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSymbol(%q)\n got: %+v\nwant: %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatSymbolRoundTrip(t *testing.T) {
	for _, s := range []string{"AAPL", "OESX:20260620:C:5200", "OESX:20260620:P:4800"} {
		c, err := ParseSymbol(s)
		if err != nil {
			t.Fatalf("ParseSymbol(%q): %v", s, err)
		}
		if got := FormatSymbol(c); got != s {
			t.Errorf("round trip %q -> %+v -> %q", s, c, got)
		}
	}
}
