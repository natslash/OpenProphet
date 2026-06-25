package interfaces

import (
	"testing"
	"time"
)

func TestQuoteTradeability(t *testing.T) {
	now := time.Now()
	fresh := func(mdt int, ageSec int) *Quote {
		return &Quote{BidPrice: 100, AskPrice: 101, MarketDataType: mdt, Timestamp: now.Add(-time.Duration(ageSec) * time.Second)}
	}
	tests := []struct {
		name   string
		q      *Quote
		maxAge time.Duration
		wantOK bool
	}{
		{"live fresh", fresh(MarketDataLive, 1), 30 * time.Second, true},
		{"delayed within window", fresh(MarketDataDelayed, 5), 60 * time.Second, true},
		{"frozen rejected", fresh(MarketDataFrozen, 0), time.Minute, false},
		{"stale beyond maxAge", fresh(MarketDataLive, 120), 30 * time.Second, false},
		{"missing prices", &Quote{MarketDataType: MarketDataLive, Timestamp: now}, time.Minute, false},
		{"nil quote", nil, time.Minute, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, warn := tt.q.Tradeability(now, tt.maxAge)
			if ok != tt.wantOK {
				t.Errorf("Tradeability ok = %v (warn=%q), want %v", ok, warn, tt.wantOK)
			}
		})
	}
}
