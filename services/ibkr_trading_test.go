package services

import "testing"

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
