package tws

import (
	"testing"
	"time"
)

func TestParseTWSDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			name:     "Epoch seconds string",
			input:    "1781806260",
			expected: time.Unix(1781806260, 0).UTC(),
		},
		{
			name:     "YYYYMMDD daily format",
			input:    "20260618",
			expected: time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Intraday spaced format",
			input:    "20260618  15:04:05",
			expected: time.Date(2026, 6, 18, 15, 4, 5, 0, time.UTC),
		},
		{
			name:     "Intraday spaced format with timezone suffix",
			input:    "20260618  15:04:05 US/Eastern",
			expected: time.Date(2026, 6, 18, 15, 4, 5, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTWSDate(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("ParseTWSDate(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
