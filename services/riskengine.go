package services

import (
	"fmt"
	"math"
	"strconv"

	"prophet-trader/configstore"
)

// riskengine.go — deterministic strike selection + trade economics + risk gating.
//
// The LLM supplies high-level CRITERIA (strategy, how far OTM the short strike
// is, wing width, expiry, risk budget); Go selects the actual strikes by
// MONEYNESS from spot, PRICES each leg with live quotes, and computes every
// number (credit, max-loss, breakevens, size, allocation) and the pass/block
// verdict — all in EUR. Selection deliberately uses moneyness + live quotes
// (reliable) rather than chain greeks (which IBKR does not populate reliably
// for the bulk OESX chain).

// QuoteFunc returns the mid price (index points) for an option leg symbol and
// whether a usable quote was found.
type QuoteFunc func(symbol string) (mid float64, ok bool)

// SpreadLeg is one selected option leg.
type SpreadLeg struct {
	Symbol string  `json:"symbol"`
	Action string  `json:"action"` // BUY | SELL
	Strike float64 `json:"strike"`
	Right  string  `json:"right"` // C | P
	Mid    float64 `json:"mid_pts"`
}

// SpreadPlan is a fully-costed, defined-risk options trade ready to place.
type SpreadPlan struct {
	Strategy   string      `json:"strategy"`
	Legs       []SpreadLeg `json:"legs"`
	Qty        int         `json:"qty"`
	Multiplier float64     `json:"multiplier"`
	WidthPts   float64     `json:"width_pts"`
	CreditPts  float64     `json:"credit_pts"`   // net per spread (>0 credit, <0 debit)
	CreditEUR  float64     `json:"credit_eur"`   // total
	MaxLossEUR float64     `json:"max_loss_eur"` // total
	Breakevens []float64   `json:"breakevens"`
	Notes      []string    `json:"notes"`
}

// SpreadCriteria is what the LLM specifies (judgment); Go resolves it to a plan.
type SpreadCriteria struct {
	Strategy      string  // iron_condor | put_credit_spread | call_credit_spread
	UnderlyingSym string  // e.g. ESTX50 (used to build leg symbols)
	Expiry        string  // YYYYMMDD
	ShortOTMPct   float64 // how far OTM the short strike is, as a fraction of spot (e.g. 0.06 = 6%)
	WidthPts      float64 // wing width in points
	Multiplier    float64 // contract multiplier (OESX = 10)
	RiskBudgetEUR float64 // size qty so max-loss ≤ this
	StrikeStep    float64 // strike grid to snap to (default 50)
}

func legSymbol(underlying, expiry, right string, strike float64) string {
	return fmt.Sprintf("%s:%s:%s:%s", underlying, expiry, right, strconv.FormatFloat(strike, 'f', -1, 64))
}

// findPricedStrike snaps target to the strike grid, then walks outward until it
// finds a strike whose leg has a usable live quote (handles illiquid/missing
// strikes). Returns the strike, its mid, and ok.
func findPricedStrike(target, step float64, right string, c SpreadCriteria, quote QuoteFunc) (float64, float64, bool) {
	if step <= 0 {
		step = 50
	}
	base := math.Round(target/step) * step
	for i := 0; i <= 8; i++ {
		cands := []float64{base + float64(i)*step}
		if i > 0 {
			cands = append(cands, base-float64(i)*step)
		}
		for _, s := range cands {
			if s <= 0 {
				continue
			}
			if m, ok := quote(legSymbol(c.UnderlyingSym, c.Expiry, right, s)); ok && m > 0 {
				return s, m, true
			}
		}
	}
	return 0, 0, false
}

// BuildSpread selects strikes by moneyness, prices them via quote, and computes
// the full costed plan. `right` codes: "P" / "C".
func BuildSpread(spot float64, quote QuoteFunc, c SpreadCriteria) (*SpreadPlan, error) {
	if spot <= 0 {
		return nil, fmt.Errorf("invalid spot %.2f", spot)
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 10
	}
	if c.StrikeStep <= 0 {
		c.StrikeStep = 50
	}
	if c.WidthPts <= 0 {
		return nil, fmt.Errorf("width must be > 0")
	}
	if c.ShortOTMPct <= 0 || c.ShortOTMPct >= 0.5 {
		return nil, fmt.Errorf("short_otm_pct must be between 0 and 0.5 (e.g. 0.06 for 6%%)")
	}

	plan := &SpreadPlan{Strategy: c.Strategy, Multiplier: c.Multiplier, WidthPts: c.WidthPts}

	// buildVertical selects a short strike at the target moneyness and a long
	// wing `width` further OTM, prices both, returns legs + net credit (points).
	buildVertical := func(right string) ([]SpreadLeg, float64, float64, error) {
		var shortTarget float64
		if right == "P" {
			shortTarget = spot * (1 - c.ShortOTMPct) // puts: below spot
		} else {
			shortTarget = spot * (1 + c.ShortOTMPct) // calls: above spot
		}
		shortStrike, shortMid, ok := findPricedStrike(shortTarget, c.StrikeStep, right, c, quote)
		if !ok {
			return nil, 0, 0, fmt.Errorf("no priced %s near %.0f (target moneyness)", right, shortTarget)
		}
		longTarget := shortStrike - c.WidthPts
		if right == "C" {
			longTarget = shortStrike + c.WidthPts
		}
		longStrike, longMid, ok := findPricedStrike(longTarget, c.StrikeStep, right, c, quote)
		if !ok {
			return nil, 0, 0, fmt.Errorf("no priced %s wing near %.0f", right, longTarget)
		}
		if longStrike == shortStrike {
			return nil, 0, 0, fmt.Errorf("%s wing resolved to the short strike (%.0f) — widen width", right, shortStrike)
		}
		credit := shortMid - longMid
		legs := []SpreadLeg{
			{Symbol: legSymbol(c.UnderlyingSym, c.Expiry, right, shortStrike), Action: "SELL", Strike: shortStrike, Right: right, Mid: shortMid},
			{Symbol: legSymbol(c.UnderlyingSym, c.Expiry, right, longStrike), Action: "BUY", Strike: longStrike, Right: right, Mid: longMid},
		}
		return legs, credit, shortStrike, nil
	}

	switch c.Strategy {
	case "put_credit_spread":
		legs, credit, shortStrike, err := buildVertical("P")
		if err != nil {
			return nil, err
		}
		plan.Legs, plan.CreditPts = legs, credit
		plan.Breakevens = []float64{shortStrike - credit}
	case "call_credit_spread":
		legs, credit, shortStrike, err := buildVertical("C")
		if err != nil {
			return nil, err
		}
		plan.Legs, plan.CreditPts = legs, credit
		plan.Breakevens = []float64{shortStrike + credit}
	case "iron_condor":
		pl, pCredit, pShort, err := buildVertical("P")
		if err != nil {
			return nil, fmt.Errorf("put side: %w", err)
		}
		cl, cCredit, cShort, err := buildVertical("C")
		if err != nil {
			return nil, fmt.Errorf("call side: %w", err)
		}
		plan.Legs = append(pl, cl...)
		plan.CreditPts = pCredit + cCredit
		plan.Breakevens = []float64{pShort - plan.CreditPts, cShort + plan.CreditPts}
	default:
		return nil, fmt.Errorf("unknown strategy %q (want iron_condor | put_credit_spread | call_credit_spread)", c.Strategy)
	}

	if plan.CreditPts <= 0 {
		return nil, fmt.Errorf("selected strikes yield a net debit (%.2f pts) — not a credit spread; check width/moneyness", plan.CreditPts)
	}

	// Max loss per spread = (width − credit) × multiplier. For an iron condor
	// only one side can be breached, so it's width − total credit.
	maxLossPerLot := (c.WidthPts - plan.CreditPts) * c.Multiplier
	if maxLossPerLot <= 0 {
		return nil, fmt.Errorf("non-positive max loss (%.2f) — check inputs", maxLossPerLot)
	}

	plan.Qty = 1
	if c.RiskBudgetEUR > 0 {
		plan.Qty = int(math.Floor(c.RiskBudgetEUR / maxLossPerLot))
	}
	if plan.Qty < 1 {
		plan.Qty = 0
		plan.Notes = append(plan.Notes, fmt.Sprintf("risk budget €%.0f is below one lot's max loss €%.0f — qty=0", c.RiskBudgetEUR, maxLossPerLot))
	}
	plan.CreditEUR = plan.CreditPts * c.Multiplier * float64(plan.Qty)
	plan.MaxLossEUR = maxLossPerLot * float64(plan.Qty)
	return plan, nil
}

// RiskVerdict is the deterministic pre-trade gate result.
type RiskVerdict struct {
	Pass    bool     `json:"pass"`
	Reasons []string `json:"reasons"`
}

// CheckSpreadRisk gates a plan against configured limits + account size.
// Spreads are defined-risk by construction; this enforces sizing/allocation.
func CheckSpreadRisk(plan *SpreadPlan, portfolioValue float64, perms configstore.Permissions) RiskVerdict {
	v := RiskVerdict{Pass: true}
	fail := func(r string) { v.Pass = false; v.Reasons = append(v.Reasons, r) }

	if plan.Qty < 1 {
		fail("qty resolves to 0 (risk budget too small for one lot)")
	}
	if portfolioValue <= 0 {
		fail("portfolio value unavailable — cannot verify allocation")
		return v
	}
	maxPos := perms.MaxPositionPct
	if maxPos <= 0 {
		maxPos = configstore.DefaultPermissions.MaxPositionPct
	}
	allocPct := plan.MaxLossEUR / portfolioValue * 100
	if allocPct > maxPos {
		fail(fmt.Sprintf("max-loss €%.0f is %.1f%% of portfolio, over the %.0f%% single-position limit", plan.MaxLossEUR, allocPct, maxPos))
	}
	if perms.MaxOrderValue > 0 && plan.MaxLossEUR > perms.MaxOrderValue {
		fail(fmt.Sprintf("max-loss €%.0f exceeds MaxOrderValue €%.0f", plan.MaxLossEUR, perms.MaxOrderValue))
	}
	if v.Pass {
		v.Reasons = append(v.Reasons, fmt.Sprintf("defined-risk; max-loss €%.0f = %.1f%% of portfolio (limit %.0f%%)", plan.MaxLossEUR, allocPct, maxPos))
	}
	return v
}
