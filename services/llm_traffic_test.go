package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"prophet-trader/interfaces"

	"github.com/stretchr/testify/assert"
)

// trafficFakeData returns a fixed spot via GetLatestQuote; rest are no-ops.
type trafficFakeData struct{ ask, bid float64 }

func (d *trafficFakeData) GetHistoricalBars(ctx context.Context, s string, a, b time.Time, tf string) ([]*interfaces.Bar, error) {
	return nil, nil
}
func (d *trafficFakeData) GetLatestBar(ctx context.Context, s string) (*interfaces.Bar, error) {
	return nil, nil
}
func (d *trafficFakeData) GetLatestQuote(ctx context.Context, s string) (*interfaces.Quote, error) {
	return &interfaces.Quote{Symbol: s, AskPrice: d.ask, BidPrice: d.bid}, nil
}
func (d *trafficFakeData) GetLatestTrade(ctx context.Context, s string) (*interfaces.Trade, error) {
	return nil, nil
}
func (d *trafficFakeData) StreamBars(ctx context.Context, syms []string) (<-chan *interfaces.Bar, error) {
	return nil, nil
}

func makeTestChain() []*interfaces.OptionContract {
	var chain []*interfaces.OptionContract
	for k := 4000; k <= 6000; k += 100 { // 21 strikes
		chain = append(chain, &interfaces.OptionContract{
			Symbol: fmt.Sprintf("ESTX50C%d", k), UnderlyingSymbol: "ESTX50",
			ContractType: "call", StrikePrice: float64(k),
			Bid: 1, Ask: 2, Delta: 0.3, ImpliedVolatility: 0.2, OpenInterest: 100, DTE: 45,
			Premium: 1.5, Volume: 10, Gamma: 0.01, Theta: -0.1, Vega: 0.2,
		})
	}
	return chain
}

func TestFormatOptionsChain_WindowsAndProjects(t *testing.T) {
	data := &trafficFakeData{ask: 5000, bid: 5000} // spot 5000 → window 4250..5750
	out := formatOptionsChain(context.Background(), data, "ESTX50", makeTestChain())

	var parsed struct {
		Spot     float64                  `json:"spot"`
		Returned int                      `json:"returned"`
		Total    int                      `json:"total"`
		Options  []map[string]interface{} `json:"options"`
	}
	assert.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, 5000.0, parsed.Spot)
	assert.Equal(t, 21, parsed.Total)
	assert.Equal(t, 15, parsed.Returned, "strikes 4300..5700 are within ±15%")
	assert.Equal(t, parsed.Returned, len(parsed.Options))

	for _, o := range parsed.Options {
		k := o["strike"].(float64)
		assert.GreaterOrEqual(t, k, 4250.0)
		assert.LessOrEqual(t, k, 5750.0)
		// Heavy fields must be projected out.
		_, hasGamma := o["gamma"]
		_, hasPremium := o["premium"]
		assert.False(t, hasGamma, "gamma should be dropped")
		assert.False(t, hasPremium, "premium should be dropped")
		// Essential fields kept.
		assert.Contains(t, o, "strike")
		assert.Contains(t, o, "bid")
	}

	// Projected payload must be smaller than the raw marshal of the full chain.
	raw, _ := json.Marshal(makeTestChain())
	assert.Less(t, len(out), len(string(raw)))
}

func TestFormatOptionsChain_NoSpotCaps(t *testing.T) {
	// data == nil → no spot → can't window → cap to maxContracts (60).
	var chain []*interfaces.OptionContract
	for k := 0; k < 100; k++ {
		chain = append(chain, &interfaces.OptionContract{
			Symbol: fmt.Sprintf("X%d", k), ContractType: "call", StrikePrice: float64(k),
		})
	}
	out := formatOptionsChain(context.Background(), nil, "X", chain)

	var parsed struct {
		Returned int     `json:"returned"`
		Spot     float64 `json:"spot"`
	}
	assert.NoError(t, json.Unmarshal([]byte(out), &parsed))
	assert.Equal(t, 0.0, parsed.Spot)
	assert.Equal(t, 60, parsed.Returned, "capped when spot is unknown")
}

func TestTruncateForHistory(t *testing.T) {
	assert.Equal(t, "short", truncateForHistory("short"))

	long := strings.Repeat("x", maxToolResultChars+500)
	got := truncateForHistory(long)
	assert.Less(t, len(got), len(long))
	assert.Contains(t, got, "truncated")
	assert.True(t, strings.HasPrefix(got, strings.Repeat("x", 100)))
}
