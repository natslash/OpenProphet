# Trading Rules

**Updated:** June 20, 2026
**Style:** The "45/7 Premium Seller" Framework

---

## Core Philosophy

- **Strategy Focus:** Premium selling (credit spreads, naked puts, covered calls, iron condors).
- **Data-Driven Edge:** We capitalize on the statistical edge found in the 0-7 day hold window (~89% win rate).
- **Tri-Agent Governance:** The CEO allocates and executes; Stratagem finds the trades; Daedalus vetoes and enforces risk limits.

---

## Capital Allocation & Position Constraints

**Rule:** Maximum 10% of portfolio per trade/instrument.
- Ensure no single trade or underlying instrument exposes the portfolio to >10% capital risk.

**Rule:** Maximum 40% deployed capital at any time.
- Prevents over-leverage and correlation wipeouts.

**Rule:** Minimum 60% cash buffer.
- Provides dry powder for new opportunities and serves as a hard buffer against tail-risk events.

---

## Entry Strategy: The 45 DTE Sweet Spot

**Rule:** Target ~45 DTE for trade entries.
- **Why:** 45 days to expiration is the mathematical sweet spot where theta (time decay) accelerates meaningfully, but gamma risk (the rate at which directional exposure changes) remains manageable.
- Collect rich premium without the violent P&L swings common in the final two weeks of an option's life.

---

## Exit Strategy: The Golden Window (0-7 Days)

**Rule:** Manage winners early (The 50% Rule).
- Target closing the position if it reaches 50% of the maximum theoretical profit.
- Take the credit, free up the capital, and remove the risk.

**Rule:** Close within ~7 days.
- **Why:** Historical ESTX50 data proves edge degradation beyond 7 days, turning negative past 14 days. 
- Try to capture the premium and exit the trade within the first 7 days. Do not hold just to squeeze out the final pennies of premium.

---

## Risk Management: The Daedalus Protocols

**Rule:** The 100% Premium Erosion Hard Stop.
- **Do not panic early:** Allow trades room to breathe. Gamma and delta will cause initial fluctuations.
- **Systemic Cut:** If the premium collected is 100% eroded (e.g., an option sold for $1.00 reaches a buy-back price of $2.00), the trade MUST be closed immediately. This caps the loss at 1x the initial credit.
- This forces nimble decision-making without emotional panic selling.

**Rule:** The Strict Time-Stop (Maximum 21 DTE).
- If a trade has not hit its profit target or stop loss, it MUST be closed or rolled no later than 21 DTE.
- **Why:** Inside 21 DTE, gamma risk spikes dramatically. A small adverse price move can wipe out weeks of accumulated theta gains instantly.
- Furthermore, holding past 14 days breaks the empirical data edge. The average losing trade was held for 24 days. Daedalus will veto holding any trade past this window.

---

## Tri-Agent Governance Workflow

1. **Stratagem** scans for premium selling opportunities primarily on the **ESTX50** index, exclusively seeking ~45 DTE setups. You may query market data, quotes, and options chains for any instrument available on IBKR (US stocks, indices like SPX/NDX, futures, forex, etc.) using the `search_contract` and `get_quote` tools. Trading focus remains on ESTX50 premium selling unless the user explicitly requests otherwise.
2. **Stratagem** drafts a proposal with exact entry limits, calculating the 50% max profit target and the 100% premium erosion hard stop.
3. **Daedalus** reviews the proposal. It verifies the 10% capital limit, the 40% portfolio cap, and validates that the stop levels are mathematical, not emotional.
4. **Daedalus** monitors open positions daily, screaming for an exit if a trade approaches day 7 or hits the 100% erosion mark.
5. **CEO** reviews the Daedalus-approved proposals and executes the trades, maintaining final say over portfolio direction.
