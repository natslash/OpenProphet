package services

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// primarySnapshotUnderlying is the index whose spot is verified and injected
// every beat. The firm trades ESTX50/OESX, so this is the price that must never
// be hallucinated.
const primarySnapshotUnderlying = "ESTX50"

// buildVerifiedSnapshot fetches authoritative IBKR data ONCE per beat — the
// underlying spot (with explicit freshness), account, and open positions — and
// renders a compact block the orchestrator and its sub-agents must treat as
// ground truth. Fetching here and reusing the result (instead of leaving each
// agent to call tools, or worse, guess) removes hallucinated/stale prices and
// minimises repeated IBKR round-trips.
func (b *AutonomousBeat) buildVerifiedSnapshot(ctx context.Context) string {
	now := time.Now()
	var sb strings.Builder
	sb.WriteString("=== VERIFIED LIVE DATA (from IBKR; DO NOT CONTRADICT OR INVENT) ===\n")
	sb.WriteString("Captured: " + now.Format(time.RFC3339) + " (CET)\n")

	// Underlying spot with explicit freshness — the single most-abused value.
	if b.data != nil {
		qctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		q, err := b.data.GetLatestQuote(qctx, primarySnapshotUnderlying)
		cancel()
		if err != nil || q == nil {
			sb.WriteString(fmt.Sprintf("%s spot: UNAVAILABLE (%v) — do NOT assume a price; treat new trades as blocked.\n", primarySnapshotUnderlying, err))
		} else {
			mid := q.BidPrice
			if q.AskPrice > 0 && q.BidPrice > 0 {
				mid = (q.BidPrice + q.AskPrice) / 2
			} else if q.AskPrice > 0 {
				mid = q.AskPrice
			}
			sb.WriteString(fmt.Sprintf("%s spot: %.2f [%s]\n", primarySnapshotUnderlying, mid, q.FreshnessLabel(now)))
		}
	}

	if b.trading != nil {
		var portfolioValue float64
		actx, cancel := context.WithTimeout(ctx, 5*time.Second)
		acc, err := b.trading.GetAccount(actx)
		cancel()
		if err == nil && acc != nil {
			portfolioValue = acc.PortfolioValue
			sb.WriteString(fmt.Sprintf("Account: portfolioValue=€%.2f cash=€%.2f buyingPower=€%.2f\n",
				acc.PortfolioValue, acc.Cash, acc.BuyingPower))
		}

		pctx, cancel2 := context.WithTimeout(ctx, 5*time.Second)
		pos, err := b.trading.GetPositions(pctx)
		cancel2()
		if err == nil {
			if len(pos) == 0 {
				sb.WriteString("Positions: FLAT (no open positions)\n")
			} else {
				// Premiums are index POINTS; cost basis and P&L are pre-computed
				// in EUR (point × contract multiplier × qty). Hand the agents the
				// EUR figures so they never multiply points by a guessed
				// multiplier — the source of past 10x risk-math errors.
				sb.WriteString("Positions (premiums in index POINTS; costBasis & P&L in EUR — USE the EUR figures, do NOT convert points yourself):\n")
				var shortEUR, longEUR, totalUPnL float64
				for _, p := range pos {
					if p.Side == "short" {
						shortEUR += p.CostBasis
					} else {
						longEUR += p.CostBasis
					}
					totalUPnL += p.UnrealizedPL
					sb.WriteString(fmt.Sprintf("  - %-5s %s qty=%.0f  entry=%.2fpts cur=%.2fpts  costBasis=€%.0f  uPnL=€%+.2f\n",
						p.Side, p.Symbol, p.Qty, p.AvgEntryPrice, p.CurrentPrice, p.CostBasis, p.UnrealizedPL))
				}
				netPremium := shortEUR - longEUR
				sb.WriteString(fmt.Sprintf("Net option premium: €%+.0f (positive = net CREDIT collected at entry)  |  Total unrealized P&L: €%+.2f\n",
					netPremium, totalUPnL))
				if acctVal := portfolioValue; acctVal > 0 {
					sb.WriteString(fmt.Sprintf("Net premium is %.1f%% of portfolio (€%.0f). Max loss of a defined-risk spread = (strike width in pts × multiplier × qty) − net credit; compute width from the strikes above.\n",
						(netPremium/acctVal)*100, acctVal))
				}
			}
		}
	}

	sb.WriteString("Rule: For any price, strike, IV, or Greek not shown above, call get_quote / get_options_chain — never guess. For P&L/credit/cost, use the EUR figures above — never recompute the multiplier.\n")
	sb.WriteString("=== END VERIFIED LIVE DATA ===")

	// Append cached, untrusted news context (qualitative only) after the
	// verified-data block, clearly delimited.
	if b.newsCache != nil {
		if nb := b.newsCache.Block(); nb != "" {
			sb.WriteString("\n\n")
			sb.WriteString(nb)
		}
	}
	return sb.String()
}

// SetNewsCache wires a background news cache whose cleaned summary is injected
// into each beat's context as untrusted qualitative input.
func (b *AutonomousBeat) SetNewsCache(c *NewsCache) {
	b.mu.Lock()
	b.newsCache = c
	b.mu.Unlock()
}

// CurrentSnapshot returns this beat's verified-data block so sub-agents
// consulted via jim_rogers reason on the same authoritative IBKR data.
func (b *AutonomousBeat) CurrentSnapshot() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentSnapshot
}
