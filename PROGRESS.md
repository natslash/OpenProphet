# PROGRESS.md — Implementation Tracker

> **Read this first every session.** Find the first 🟡 or ⬜ item — that's where we are. Do only that step, test it, then stop and confirm before the next.
>
> **Source of truth for detail:** `IBKR_MIGRATION_PLAN_v2.md` (phases + test criteria) and `CLAUDE.md` (specs, interfaces, wrapper guide).
>
> **Rules:**
> - One step = one commit = one testable change
> - Mark ✅ only after the test passes
> - Never skip ahead
> - Order-placing paths stay manual / human-in-the-loop
> - Paper only (port 4002) until Phase 6
>
> **Legend:** ✅ done · 🟡 in progress / awaiting verification · ⬜ not started

---

## Where we are right now

**Phase 1.1.** Phase 0 is complete (socket sanity verified against IB Gateway v187, code review fixes applied). **Next action:** Define `BrokerService` + `MarketDataService` in `interfaces/` from the methods controllers actually call (drafts in CLAUDE.md).

---

## Phase 0 — Baseline & socket sanity

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 0.1 | Branch `feature/ibkr-porting` off `main`; planning docs in repo | ✅ | 2026-03-22 | 2149d18 |
| 0.2 | IB Gateway paper running on 4002; API socket clients enabled, 127.0.0.1 trusted; record server version | ✅ | 2026-06-18 | Verified v187 |
| 0.3 | `cmd/twscheck` — TCP connect + v100+ handshake, print server version (no `startApi`, no orders) | ✅ | 2026-06-18 | Handshake OK |

**Test criteria**
- **0.2:** Gateway logs in; API > Settings shows socket clients enabled, port 4002, trusted IP 127.0.0.1.
- **0.3:** `go run ./cmd/twscheck` prints `handshake succeeded` + a server version from your Gateway. *(Code complete + sandbox-tested; awaiting this real-Gateway run.)*

---

## Phase 1 — Broker abstraction seam (no behaviour change)

Introduce the interface, make existing Alpaca satisfy it, select with `BROKER=`. Proves the seam with zero regression before any IBKR code.

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 1.1 | Define `BrokerService` + `MarketDataService` in `interfaces/` from the methods controllers actually call (drafts in CLAUDE.md) | ⬜ | | |
| 1.2 | Make existing Alpaca code satisfy the interfaces — thin adapter, no logic change | ⬜ | | |
| 1.3 | Wire controllers + MCP to the interface, selected by `BROKER=alpaca` | ⬜ | | |

**Test criteria**
- **1.1:** Compiles; interfaces cover every call site.
- **1.2:** App still trades on Alpaca paper; endpoints behave identically.
- **1.3:** Full autonomous beat on Alpaca, now routed through the interface — zero regression.

---

## Phase 2 — TWS wrapper (`tws/`), protocol only, NO orders

Pure protocol. Codec is unit-testable against recorded bytes (Fabro-eligible per CLAUDE.md). Builds on the `cmd/twscheck` handshake.

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 2.1 | `tws/client.go` — promote the twscheck handshake; add `startApi`, capture `nextValidId` | ✔︎ | | | ## DU5894187 / first order id 1
| 2.2 | `tws/encoder.go` + `tws/decoder.go` + `tws/constants.go` — framing both ways; round-trip `reqCurrentTimeInMillis()` | ⬜ | | |
| 2.3 | `tws/dispatcher.go` (reqId→chan) + `tws/order_id_manager.go` (seed + atomic next) | ⬜ | | |
| 2.4 | `tws/contract.go` + `reqContractDetails` for OESX (ESTX50) | ⬜ | | |
| 2.5 | Market-data subscribe; parse ticks incl. **Decimal** sizes (types 5, 71) | ⬜ | | |

**Test criteria**
- **2.1:** Handshake returns server version + first valid order id.
- **2.2:** `reqCurrentTimeInMillis()` round-trips to epoch ms; table-driven encoder/decoder unit tests pass.
- **2.3:** Two concurrent requests resolve to the right callers.
- **2.4:** Returns conId, multiplier, expiries for a real OESX contract.
- **2.5:** Live OESX bid/ask/last ticks print; sizes decode as decimals.

---

## Phase 3 — IBKR read-only services

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 3.1 | `services/ibkr_market_data.go` implements `MarketDataService` over `tws/` | ⬜ | | |
| 3.2 | `services/ibkr_broker.go` read paths: account, positions, open orders (filter de-activated) | ⬜ | | |

**Test criteria**
- **3.1:** Quotes/Greeks for OESX match the TWS UI.
- **3.2:** Account/positions match the TWS paper account exactly; **no order placed.**

---

## Phase 4 — Order execution (paper, manual, tightly gated)

Human-in-the-loop. Not a candidate for autonomous orchestration — this path can send orders.

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 4.1 | `placeOrder` / `cancelOrder` + `orderStatus` / `openOrder` callbacks via the dispatcher | ⬜ | | |
| 4.2 | Bracket orders (parent + TP + SL, OCA) | ⬜ | | |
| 4.3 | `BROKER=ibkr` end-to-end autonomous beat on paper | ⬜ | | |

**Test criteria**
- **4.1:** 1-lot OESX **paper** order places, fills, reconciles.
- **4.2:** Parent + TP + SL submit atomically; OCA behaves on partial fill.
- **4.3:** Agent wakes → assesses → places/manages a paper trade → sleeps.

---

## Phase 5 — Cutover & instrument-agnostic polish

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 5.1 | Contract mapping for US/EU stocks + futures alongside OESX options | ⬜ | | |
| 5.2 | News/feeds → European sources; remove the dead `reqFundamentalData` path | ⬜ | | |
| 5.3 | Default `BROKER=ibkr`; demote Alpaca to fallback | ⬜ | | |

**Test criteria**
- **5.1:** Each instrument type round-trips contractDetails + a paper order.
- **5.2:** `news_service` returns EU sources; no calls to removed fundamentals APIs.
- **5.3:** Clean autonomous run on IBKR paper from a cold start.

---

## Phase 6 — Later / optional

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 6.1 | Live (port 4001) behind an explicit double-confirm guard | ⬜ | | |
| 6.2 | Java backend migration (optional, separate effort) | ⬜ | | |
| 6.3 | Merge the Claude Code CLI swap track | ⬜ | | |

---

## Notes

_Add dated notes when something unexpected happens, a step needs rework, or a design decision changes._

```
YYYY-MM-DD | Step X.X | Note text here
```
