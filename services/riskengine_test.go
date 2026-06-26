package services

import (
	"math"
	"testing"

	"prophet-trader/configstore"
)

// stubQuotes prices specific leg symbols; everything else is "no quote".
func stubQuotes(prices map[string]float64) QuoteFunc {
	return func(symbol string) (float64, bool) {
		if m, ok := prices[symbol]; ok {
			return m, true
		}
		return 0, false
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestBuildPutCreditSpread(t *testing.T) {
	// spot 6260, 6% OTM short put -> 5884.4 -> snap/50 -> 5900; wing 5800.
	q := stubQuotes(map[string]float64{
		"ESTX50:20260821:P:5900": 30,
		"ESTX50:20260821:P:5800": 20,
	})
	p, err := BuildSpread(6260, q, SpreadCriteria{
		Strategy: "put_credit_spread", UnderlyingSym: "ESTX50", Expiry: "20260821",
		ShortOTMPct: 0.06, WidthPts: 100, Multiplier: 10, RiskBudgetEUR: 2000, StrikeStep: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Legs[0].Action != "SELL" || p.Legs[0].Strike != 5900 || p.Legs[1].Strike != 5800 {
		t.Fatalf("legs wrong: %+v", p.Legs)
	}
	if !approx(p.CreditPts, 10) {
		t.Errorf("credit pts = %.2f, want 10", p.CreditPts)
	}
	if p.Qty != 2 { // floor(2000 / ((100-10)*10=900)) = 2
		t.Errorf("qty = %d, want 2", p.Qty)
	}
	if !approx(p.MaxLossEUR, 1800) || !approx(p.CreditEUR, 200) {
		t.Errorf("maxLoss=%.0f credit=%.0f, want 1800/200", p.MaxLossEUR, p.CreditEUR)
	}
	if len(p.Breakevens) != 1 || !approx(p.Breakevens[0], 5890) {
		t.Errorf("breakeven = %v, want [5890]", p.Breakevens)
	}
}

func TestBuildIronCondor(t *testing.T) {
	// spot 6260: short put 5900, short call 6650; wings 5800 / 6750.
	q := stubQuotes(map[string]float64{
		"ESTX50:20260821:P:5900": 30, "ESTX50:20260821:P:5800": 20,
		"ESTX50:20260821:C:6650": 28, "ESTX50:20260821:C:6750": 18,
	})
	p, err := BuildSpread(6260, q, SpreadCriteria{
		Strategy: "iron_condor", UnderlyingSym: "ESTX50", Expiry: "20260821",
		ShortOTMPct: 0.06, WidthPts: 100, Multiplier: 10, RiskBudgetEUR: 2000, StrikeStep: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Legs) != 4 {
		t.Fatalf("want 4 legs, got %d", len(p.Legs))
	}
	if !approx(p.CreditPts, 20) { // put 10 + call 10
		t.Errorf("credit pts = %.2f, want 20", p.CreditPts)
	}
	if p.Qty != 2 { // floor(2000 / ((100-20)*10=800)) = 2
		t.Errorf("qty = %d, want 2", p.Qty)
	}
	if !approx(p.MaxLossEUR, 1600) {
		t.Errorf("maxLossEUR = %.0f, want 1600", p.MaxLossEUR)
	}
	if len(p.Breakevens) != 2 || !approx(p.Breakevens[0], 5880) || !approx(p.Breakevens[1], 6670) {
		t.Errorf("breakevens = %v, want [5880 6670]", p.Breakevens)
	}
}

func TestFindPricedStrikeWalksToLiquid(t *testing.T) {
	// Short target snaps to 5900 but only 5850 has a quote -> should walk to 5850.
	q := stubQuotes(map[string]float64{
		"ESTX50:20260821:P:5850": 32,
		"ESTX50:20260821:P:5750": 21,
	})
	p, err := BuildSpread(6260, q, SpreadCriteria{
		Strategy: "put_credit_spread", UnderlyingSym: "ESTX50", Expiry: "20260821",
		ShortOTMPct: 0.06, WidthPts: 100, Multiplier: 10, RiskBudgetEUR: 2000, StrikeStep: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Legs[0].Strike != 5850 || p.Legs[1].Strike != 5750 {
		t.Fatalf("expected walk to priced strikes 5850/5750, got %+v", p.Legs)
	}
}

func TestCheckSpreadRisk(t *testing.T) {
	plan := &SpreadPlan{Qty: 2, MaxLossEUR: 1680}
	if v := CheckSpreadRisk(plan, 100000, configstore.DefaultPermissions); !v.Pass {
		t.Errorf("1.68%% should pass: %v", v.Reasons)
	}
	if v := CheckSpreadRisk(plan, 10000, configstore.DefaultPermissions); v.Pass {
		t.Error("16.8%% should FAIL the 15%% limit")
	}
	if v := CheckSpreadRisk(&SpreadPlan{Qty: 0}, 100000, configstore.DefaultPermissions); v.Pass {
		t.Error("qty 0 should fail")
	}
}
