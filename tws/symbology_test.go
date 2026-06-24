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
			name: "eu stock",
			in:   "EU:DTE",
			want: Contract{Symbol: "DTE", SecType: Stock, Exchange: "SMART", Currency: "EUR"},
		},
		{
			name: "future",
			in:   "FUT:OESX:20260619",
			want: Contract{Symbol: "OESX", SecType: Future, Exchange: "EUREX", Currency: "EUR", LastTradeDateOrContractMonth: "20260619"},
		},
		{
			name: "estx50 call",
			in:   "ESTX50:20260619:C:6325",
			want: Contract{
				Symbol: "ESTX50", SecType: Option, Exchange: "EUREX", Currency: "EUR",
				LastTradeDateOrContractMonth: "20260619", Strike: 6325, Right: "C",
				Multiplier: "10", TradingClass: "OESX",
			},
		},
		{
			name: "estx50 put lowercase right",
			in:   "ESTX50:20260619:p:4800",
			want: Contract{
				Symbol: "ESTX50", SecType: Option, Exchange: "EUREX", Currency: "EUR",
				LastTradeDateOrContractMonth: "20260619", Strike: 4800, Right: "P",
				Multiplier: "10", TradingClass: "OESX",
			},
		},
		{name: "empty", in: "", wantErr: true},
		{name: "estx50 wrong arity", in: "ESTX50:20260619:C", wantErr: true},
		{name: "estx50 bad right", in: "ESTX50:20260619:X:5200", wantErr: true},
		{name: "estx50 bad strike", in: "ESTX50:20260619:C:abc", wantErr: true},
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
			if got.ConId != tc.want.ConId || got.Symbol != tc.want.Symbol || got.SecType != tc.want.SecType ||
				got.Exchange != tc.want.Exchange || got.Currency != tc.want.Currency ||
				got.LastTradeDateOrContractMonth != tc.want.LastTradeDateOrContractMonth ||
				got.Strike != tc.want.Strike || got.Right != tc.want.Right ||
				got.Multiplier != tc.want.Multiplier || got.TradingClass != tc.want.TradingClass {
				t.Fatalf("ParseSymbol(%q)\n got: %+v\nwant: %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatSymbolRoundTrip(t *testing.T) {
	for _, s := range []string{"AAPL", "EU:DTE", "FUT:OESX:20260619", "ESTX50:20260619:C:6325", "ESTX50:20260619:P:4800"} {
		c, err := ParseSymbol(s)
		if err != nil {
			t.Fatalf("ParseSymbol(%q): %v", s, err)
		}
		if got := FormatSymbol(c); got != s {
			t.Errorf("round trip %q -> %+v -> %q", s, c, got)
		}
	}
}
