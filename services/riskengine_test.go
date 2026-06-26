package services

import (
	"math"
	"testing"

	"prophet-trader/configstore"
	"prophet-trader/interfaces"
)

func synthChain() []*interfaces.OptionContract {
	mk := func(right string, strike, delta, bid, ask float64) *interfaces.OptionContract {
		return &interfaces.OptionContract{ContractType: right, StrikePrice: strike, Delta: delta, Bid: bid, Ask: ask}
	}
	return []*interfaces.OptionContract{
		mk("put", 5750, -0.10, 18, 20), // mid 19
		mk("put", 5850, -0.16, 24, 26), // mid 25  <- 16-delta short put
		mk("put", 5950, -0.25, 34, 36), // mid 35
		mk("call", 6550, 0.25, 40, 42), // mid 41
		mk("call", 6650, 0.16, 28, 30), // mid 29  <- 16-delta short call
		mk("call", 6750, 0.10, 18, 20), // mid 19
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestSelectByDelta(t *testing.T) {
	c := synthChain()
	if got := selectByDelta(c, "put", 0.16); got == nil || got.StrikePrice != 5850 {
		t.Fatalf("16-delta put: got %+v, want strike 5850", got)
	}
	if got := selectByDelta(c, "call", 0.16); got == nil || got.StrikePrice != 6650 {
		t.Fatalf("16-delta call: got %+v, want strike 6650", got)
	}
}

func TestBuildPutCreditSpread(t *testing.T) {
	p, err := BuildSpread(synthChain(), SpreadCriteria{
		Strategy: "put_credit_spread", UnderlyingSym: "ESTX50", Expiry: "20260821",
		ShortDelta: 0.16, WidthPts: 100, Multiplier: 10, RiskBudgetEUR: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Legs) != 2 || p.Legs[0].Action != "SELL" || p.Legs[0].Strike != 5850 || p.Legs[1].Strike != 5750 {
		t.Fatalf("legs wrong: %+v", p.Legs)
	}
	if !approx(p.CreditPts, 6) { // 25 - 19
		t.Errorf("credit pts = %.2f, want 6", p.CreditPts)
	}
	if p.Qty != 2 { // floor(2000 / ((100-6)*10=940)) = 2
		t.Errorf("qty = %d, want 2", p.Qty)
	}
	if !approx(p.MaxLossEUR, 1880) { // 940 * 2
		t.Errorf("maxLossEUR = %.0f, want 1880", p.MaxLossEUR)
	}
	if !approx(p.CreditEUR, 120) { // 6*10*2
		t.Errorf("creditEUR = %.0f, want 120", p.CreditEUR)
	}
	if len(p.Breakevens) != 1 || !approx(p.Breakevens[0], 5844) {
		t.Errorf("breakeven = %v, want [5844]", p.Breakevens)
	}
	if p.Legs[0].Symbol != "ESTX50:20260821:P:5850" {
		t.Errorf("symbol = %q", p.Legs[0].Symbol)
	}
}

func TestBuildIronCondor(t *testing.T) {
	p, err := BuildSpread(synthChain(), SpreadCriteria{
		Strategy: "iron_condor", UnderlyingSym: "ESTX50", Expiry: "20260821",
		ShortDelta: 0.16, WidthPts: 100, Multiplier: 10, RiskBudgetEUR: 2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Legs) != 4 {
		t.Fatalf("want 4 legs, got %d", len(p.Legs))
	}
	if !approx(p.CreditPts, 16) { // put 6 + call 10
		t.Errorf("credit pts = %.2f, want 16", p.CreditPts)
	}
	if p.Qty != 2 { // floor(2000 / ((100-16)*10=840)) = 2
		t.Errorf("qty = %d, want 2", p.Qty)
	}
	if !approx(p.MaxLossEUR, 1680) {
		t.Errorf("maxLossEUR = %.0f, want 1680", p.MaxLossEUR)
	}
	if len(p.Breakevens) != 2 || !approx(p.Breakevens[0], 5834) || !approx(p.Breakevens[1], 6666) {
		t.Errorf("breakevens = %v, want [5834 6666]", p.Breakevens)
	}
}

func TestCheckSpreadRisk(t *testing.T) {
	plan := &SpreadPlan{Qty: 2, MaxLossEUR: 1680}
	if v := CheckSpreadRisk(plan, 100000, configstore.DefaultPermissions); !v.Pass {
		t.Errorf("1.68%% of portfolio should pass: %v", v.Reasons)
	}
	if v := CheckSpreadRisk(plan, 10000, configstore.DefaultPermissions); v.Pass {
		t.Error("16.8%% of portfolio should FAIL the 15%% limit")
	}
	if v := CheckSpreadRisk(&SpreadPlan{Qty: 0}, 100000, configstore.DefaultPermissions); v.Pass {
		t.Error("qty 0 should fail")
	}
}
