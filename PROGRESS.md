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
> - **End state is IBKR-only** — Alpaca stays as working scaffolding during the build and is *deleted* at cutover, not kept as a permanent fallback
>
> **Legend:** ✅ done · 🟡 in progress / awaiting verification · ⬜ not started · ~~struck~~ no longer relevant

---

## Where we are right now

**Phase 2.4.** Phase 2.3 complete (dispatcher and order ID manager implemented and tested). **Next action:** `tws/contract.go` + `reqContractDetails` for OESX (ESTX50).

---

## Phase 0 — Baseline & socket sanity

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 0.1 | Branch + planning docs in repo | ✅ | 2026-03-22 | 2149d18 |
| 0.2 | IB Gateway paper running on 4002; record server version | ✅ | 2026-06-18 | v187, acct DU5894187 |
| 0.3 | `cmd/twscheck` — TCP connect + v100+ handshake (no `startApi`, no orders) | ✅ | 2026-06-18 | 09286ff |

---

## Phase 1 — ~~Broker abstraction seam~~ → ALREADY EXISTS (no work needed)

The seam was already built upstream: `interfaces.TradingService` and `interfaces.DataService` are defined in `interfaces/trading.go` and are consumed by the controllers and managers (`order_controller`, `intelligence_controller`, `position_manager`, etc.). The Alpaca services already implement them. **Decision: Option 1** — IBKR implements these *existing* interfaces; we do **not** introduce new `BrokerService`/`MarketDataService` types.

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 1.1 | ~~Define `BrokerService` + `MarketDataService`~~ | ✅ n/a | — | seam pre-exists as `interfaces.TradingService`/`DataService` |
| 1.2 | ~~Make Alpaca satisfy the interfaces~~ | ✅ n/a | — | Alpaca already implements them (proven: app compiles & runs) |
| 1.3 | ~~Wire controllers/MCP to the interface~~ | ✅ n/a | — | controllers already depend on the interfaces, not concrete Alpaca |

**Optional close-out (not done, low priority):** add explicit compile-time assertions in `services/interface_guard.go` (`var _ interfaces.TradingService = (*AlpacaTradingService)(nil)` …). Not required; its value is catching an incomplete IBKR impl at build time — we'll add the IBKR assertions when those structs land in Phase 3.

---

## Phase 2 — TWS wrapper (`tws/`), protocol only, NO orders

Pure protocol. Codec is unit-testable against recorded bytes (Fabro-eligible per CLAUDE.md).

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 2.1 | `tws/tws_client.go` — handshake, `startApi`, capture `nextValidId` (+ `managedAccounts`) | ✅ | 2026-06-18 | f174446, 1486683 |
| 2.2 | `tws/encoder.go` + `tws/decoder.go` + `tws/constants.go` — framing both ways; round-trip `reqCurrentTimeInMillis()` | ✅ | 2026-06-18 | pending |
| 2.3 | `tws/dispatcher.go` (reqId→chan) + `tws/order_id_manager.go` (seed + atomic next) | ✅ | 2026-06-18 | pending |
| 2.4 | `tws/contract.go` + `reqContractDetails` for OESX (ESTX50) | ⬜ | | |
| 2.5 | Market-data subscribe; parse ticks incl. **Decimal** sizes (types 5, 71) | ⬜ | | |

**2.1 close-out notes:** connect blocks until both `nextValidId` and `managedAccounts`; `AsyncErrorCallback` routes post-connect errors (pre-connect errors stay fatal on `errCh`); single-write framing; `splitFields` preserves trailing empty fields. Unit tests: `TestSplitFields`, `TestWriteFrame`, `TestHandleMessage_AsyncError`. Verified live: server 187, account DU5894187, first valid order id 1.

**Test criteria**
- **2.2:** `reqCurrentTimeInMillis()` round-trips to epoch ms; table-driven encoder/decoder tests pass.
- **2.3:** Two concurrent requests resolve to the right callers.
- **2.4:** Returns conId, multiplier, expiries for a real OESX contract.
- **2.5:** Live OESX bid/ask/last ticks print; sizes decode as decimals.

---

## Phase 3 — IBKR read-only services

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 3.1 | `services/ibkr_data.go` implements `interfaces.DataService` over `tws/` (+ assert) | ⬜ | | |
| 3.2 | `services/ibkr_trading.go` read paths: account, positions, open orders (filter de-activated) | ⬜ | | |

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

## Phase 5 — Cutover to IBKR-only

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 5.1 | Contract mapping for US/EU stocks + futures alongside OESX options | ⬜ | | |
| 5.2 | News/feeds → European sources; remove the dead `reqFundamentalData` path | ⬜ | | |
| 5.3 | Switch wiring to IBKR and **delete the Alpaca services** (IBKR-only; the `BROKER=` flag was only a temporary A/B aid during the build) | ⬜ | | |

**Test criteria**
- **5.1:** Each instrument type round-trips contractDetails + a paper order.
- **5.2:** `news_service` returns EU sources; no calls to removed fundamentals APIs.
- **5.3:** Clean autonomous run on IBKR paper from a cold start, Alpaca code gone.

---

## Phase 6 — Later / optional

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 6.1 | Live (port 4001) behind an explicit double-confirm guard | ⬜ | | |
| 6.2 | Java backend migration (optional, separate effort) | ⬜ | | |
| 6.3 | Merge the Claude Code CLI swap track | ⬜ | | |

---

## Notes

```
2026-06-18 | Phase 0   | Verified live against IB Gateway: server v187, account DU5894187, first valid order id 1.
2026-06-18 | Phase 1   | Reconciled: broker seam already existed upstream (interfaces.TradingService/DataService, consumed by controllers). Option 1 adopted — IBKR implements existing interfaces, no new BrokerService/MarketDataService. Phase 1 steps struck as not-applicable.
2026-06-18 | Step 2.1  | Code-review fixes on fix/tws-client-improvements (commit 1486683): wait for nextValidId+managedAccounts, AsyncErrorCallback, single-write framing, splitFields trailing-empty fix; added unit tests.
2026-06-18 | Docs      | IBKR_MIGRATION_PLAN_v2.md still describes Phase 1 as "define the seam / seam does not exist" — stale; needs the same Option-1 reconciliation. File naming: actual files are tws/tws_client.go and cmd/twsconnect/twsconnect_main.go (plan/CLAUDE say tws/client.go); harmless.
2026-06-18 | Step 2.2  | Codec layer built (constants, wrapper interface, encoder, decoder) on the fix/tws-client-improvements branch. Integrated directly into tws_client.go. Verified against IB Gateway.
2026-06-18 | Branching | Merged fix/tws-client-improvements (containing both Phase 2.1 fixes and Phase 2.2 feature work) back into the main migration branch feature/ibkr-porting, and deleted the fix branch to maintain clean semantics moving forward.
2026-06-18 | Step 2.3  | Implemented Dispatcher and OrderIdManager. Integrated OrderIdManager into tws_client.go to replace manual nextOrderID fields. Unit tests passed, manual test verified NextOrderId() seeding works.
```
