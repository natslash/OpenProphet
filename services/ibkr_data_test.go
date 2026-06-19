package services

import (
	"testing"
	"time"
)

func TestCalculateDuration(t *testing.T) {
	base := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		dur     time.Duration
		barSize string
		want    string
	}{
		{"daily 5d", 5 * 24 * time.Hour, "1 day", "5 D"},
		{"1min sub-day 1h", time.Hour, "1 min", "3600 S"},
		{"5min sub-day 30m", 30 * time.Minute, "5 mins", "1800 S"},
		{"1min tiny window floored to 60s", 10 * time.Second, "1 min", "60 S"},
		{"1min multi-day clamped to 2D", 10 * 24 * time.Hour, "1 min", "2 D"},
		{"5min multi-day clamped to 5D", 10 * 24 * time.Hour, "5 mins", "5 D"},
		{"hour sub-day uses seconds", 3 * time.Hour, "1 hour", "10800 S"},
		{"daily sub-day rounds to 1D", time.Hour, "1 day", "1 D"},
		{"daily long maps to years", 800 * 24 * time.Hour, "1 day", "3 Y"},
		{"zero window defensive", 0, "1 min", "1 D"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateDuration(base.Add(-tc.dur), base, tc.barSize)
			if got != tc.want {
				t.Errorf("calculateDuration(dur=%v, %q) = %q, want %q", tc.dur, tc.barSize, got, tc.want)
			}
		})
	}
}
