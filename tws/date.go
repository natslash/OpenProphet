package tws

import (
	"strconv"
	"strings"
	"time"
)

// ParseTWSDate parses dates returned by TWS into UTC time.Time.
// It handles:
// - Epoch seconds strings ("1781806260")
// - YYYYMMDD ("20260618")
// - YYYYMMDD  HH:mm:ss ("20260618  15:04:05") with optional timezone
func ParseTWSDate(dateStr string) time.Time {
	dateStr = strings.TrimSpace(dateStr)
	
	// Fast path: if it's 8 digits exactly, it's YYYYMMDD
	if len(dateStr) == 8 {
		if parsed, err := time.Parse("20060102", dateStr); err == nil {
			return parsed.UTC()
		}
	}

	// Fast path: if it's all digits and len > 8, it's probably epoch seconds (formatDate=2)
	// Example: 1781806260
	if isAllDigits(dateStr) {
		if epochSecs, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
			return time.Unix(epochSecs, 0).UTC()
		}
	}

	// Slow path: Intraday format string (YYYYMMDD  HH:mm:ss)
	// Some versions of TWS return a timezone suffix like "20260618  15:04:05 US/Eastern"
	// We'll try to parse the base time format. If it fails we'll fallback to Now.
	if len(dateStr) >= 17 {
		// Try the exact known layout first
		if parsed, err := time.Parse("20060102  15:04:05", dateStr[:18]); err == nil {
			return parsed.UTC()
		}
	}

	return time.Now().UTC()
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
