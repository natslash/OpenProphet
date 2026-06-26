package services

import (
	"fmt"
	"math"
	"strconv"

	"prophet-trader/configstore"
	"prophet-trader/interfaces"
)

// riskengine.go — deterministic strike selection + trade economics + risk gating.
//
// The LLM supplies high-level CRITERIA (strategy, target short-delta, wing
// width, expiry, risk budget); Go selects the actual strikes from the live
// IBKR chain and computes every number (credit, max-loss, breakevens, size,
// allocation) and the pass/block verdict. The LLM never picks a strike or does
// arithmetic — it only decides the criteria and whether to take the costed plan.

// SpreadLeg is one selected option leg.
type SpreadLeg struct {
	Symbol string  `json:"symbol"`
	Action string  `json:"action"` // BUY | SELL
	Strike float64 `json:"strike"`
	Right  string  `json:"right"` // C | P
	Mid    float64 `json:"mid_pts"`
	Delta  float64 `json:"delta"`
}

// SpreadPlan is a fully-costed, defined-risk options trade ready to place.
type SpreadPlan struct {
	Strategy   string      `json:"strategy"`
	Legs       []SpreadLeg `json:"legs"`
	Qty        int         `json:"qty"`
	Multiplier float64     `json:"multiplier"`
	WidthPts   float64     `json:"width_pts"`
	CreditPts  float64     `json:"credit_pts"`  // net per spread (>0 credit, <0 debit)
	CreditEUR  float64     `json:"credit_eur"`  // total
	MaxLossEUR float64     `json:"max_loss_eur"` // total
	Breakevens []float64   `json:"breakevens"`
	EstPOPPct  float64     `json:"est_pop_pct"` // rough probability-of-profit
	Notes      []string    `json:"notes"`
}

// SpreadCriteria is what the LLM specifies (judgment); Go resolves it to a plan.
type SpreadCriteria struct {
	Strategy      string  // iron_condor | put_credit_spread | call_credit_spread
	UnderlyingSym string  // e.g. ESTX50 (used to build leg symbols)
	Expiry        string  // YYYYMMDD
	ShortDelta    float64 // target |delta| for the short strike(s), e.g. 0.16
	WidthPts      float64 // wing width in points
	Multiplier    float64 // contract multiplier (OESX = 10)
	RiskBudgetEUR float64 // size qty so max-loss ≤ this
}

func optMid(c *interfaces.OptionContract) float64 {
	if c.Bid > 0 && c.Ask > 0 {
		return (c.Bid + c.Ask) / 2
	}
	if c.Premium > 0 {
		return c.Premium
	}
	return math.Max(c.Bid, c.Ask)
}

// selectByDelta returns the contract of the given right whose |delta| is closest
// to targetAbs (and has a usable price).
func selectByDelta(chain []*interfaces.OptionContract, right string, targetAbs float64) *interfaces.OptionContract {
	var best *interfaces.OptionContract
	bestDiff := math.MaxFloat64
	for _, c := range chain {
		if c.ContractType != right || optMid(c) <= 0 {
			continue
		}
		diff := math.Abs(math.Abs(c.Delta) - targetAbs)
		if diff < bestDiff {
			bestDiff, best = diff, c
		}
	}
	return best
}

// findStrike returns the contract of the given right closest to strike (with a price).
func findStrike(chain []*interfaces.OptionContract, right string, strike float64) *interfaces.OptionContract {
	var best *interfaces.OptionContract
	bestDiff := math.MaxFloat64
	for _, c := range chain {
		if c.ContractType != right || optMid(c) <= 0 {
			continue
		}
		diff := math.Abs(c.StrikePrice - strike)
		if diff < bestDiff {
			bestDiff, best = diff, c
		}
	}
	return best
}

func legSymbol(underlying, expiry, right string, strike float64) string {
	return fmt.Sprintf("%s:%s:%s:%s", underlying, expiry, right, strconv.FormatFloat(strike, 'f', -1, 64))
}

// BuildSpread selects strikes from the chain and computes the full costed plan.
// "right" is "call" / "put" matching OptionContract.ContractType.
func BuildSpread(chain []*interfaces.OptionContract, c SpreadCriteria) (*SpreadPlan, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty options chain")
	}
	if c.Multiplier <= 0 {
		c.Multiplier = 10 // OESX default
	}
	if c.WidthPts <= 0 {
		return nil, fmt.Errorf("width must be > 0")
	}
	if c.ShortDelta <= 0 || c.ShortDelta >= 1 {
		return nil, fmt.Errorf("short_delta must be between 0 and 1")
	}

	plan := &SpreadPlan{Strategy: c.Strategy, Multiplier: c.Multiplier, WidthPts: c.WidthPts}
	rights := map[string]string{"P": "put", "C": "call"}

	// buildVertical selects a short strike at target delta and a long wing
	// `width` further OTM. Returns the two legs and the net credit (points).
	buildVertical := func(rightCode string) ([]SpreadLeg, float64, float64, error) {
		ct := rights[rightCode]
		short := selectByDelta(chain, ct, c.ShortDelta)
		if short == nil {
			return nil, 0, 0, fmt.Errorf("no %s found near delta %.2f", ct, c.ShortDelta)
		}
		longStrike := short.StrikePrice - c.WidthPts // puts: wing below
		if rightCode == "C" {
			longStrike = short.StrikePrice + c.WidthPts // calls: wing above
		}
		long := findStrike(chain, ct, longStrike)
		if long == nil {
			return nil, 0, 0, fmt.Errorf("no %s wing near strike %.0f", ct, longStrike)
		}
		credit := optMid(short) - optMid(long)
		legs := []SpreadLeg{
			{Symbol: legSymbol(c.UnderlyingSym, c.Expiry, rightCode, short.StrikePrice), Action: "SELL", Strike: short.StrikePrice, Right: rightCode, Mid: optMid(short), Delta: short.Delta},
			{Symbol: legSymbol(c.UnderlyingSym, c.Expiry, rightCode, long.StrikePrice), Action: "BUY", Strike: long.StrikePrice, Right: rightCode, Mid: optMid(long), Delta: long.Delta},
		}
		return legs, credit, short.StrikePrice, nil
	}

	switch c.Strategy {
	case "put_credit_spread":
		legs, credit, shortStrike, err := buildVertical("P")
		if err != nil {
			return nil, err
		}
		plan.Legs = legs
		plan.CreditPts = credit
		plan.Breakevens = []float64{shortStrike - credit}
		plan.EstPOPPct = (1 - c.ShortDelta) * 100
	case "call_credit_spread":
		legs, credit, shortStrike, err := buildVertical("C")
		if err != nil {
			return nil, err
		}
		plan.Legs = legs
		plan.CreditPts = credit
		plan.Breakevens = []float64{shortStrike + credit}
		plan.EstPOPPct = (1 - c.ShortDelta) * 100
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
		plan.EstPOPPct = (1 - 2*c.ShortDelta) * 100
	default:
		return nil, fmt.Errorf("unknown strategy %q (want iron_condor | put_credit_spread | call_credit_spread)", c.Strategy)
	}

	if plan.CreditPts <= 0 {
		return nil, fmt.Errorf("selected strikes yield a net debit (%.2f pts) — not a credit spread; widen the gap or check the chain", plan.CreditPts)
	}

	// Max loss per spread = (width − credit) points × multiplier. (For an iron
	// condor only one side can be breached, so it's width − total credit.)
	maxLossPerLot := (c.WidthPts - plan.CreditPts) * c.Multiplier
	if maxLossPerLot <= 0 {
		return nil, fmt.Errorf("non-positive max loss (%.2f) — check inputs", maxLossPerLot)
	}

	// Size to the risk budget.
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

// CheckSpreadRisk gates a plan against the configured limits + account size.
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
