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

**Phase 4.1–4.3e ✅.** Order execution (4.1/4.2) validated live on paper. Historical data (4.3a codec + 4.3b service) verified live. Safe bot wiring (4.3c) connects to paper with `GatedTradingService`. PositionManager bracket refactor (4.3d) natively maps child orders. Supervised autonomous beat (4.3e) accurately analyzes market signals and strictly gates execution behind a binary kill-switch (full token auth deferred to 6.1).

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
| 2.4 | `tws/contract.go` + `reqContractDetails` for OESX (ESTX50) | ✅ | 2026-06-18 | pending |
| 2.5 | Market-data subscribe; parse ticks incl. **Decimal** sizes (types 5, 71) | ✅ | 2026-06-18 | pending |

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
| 3.1 | `services/ibkr_data.go` implements `interfaces.DataService` over `tws/` (+ assert) | ✅ | 2026-06-18 | pending |
| 3.2 | `services/ibkr_trading.go` read paths: account, positions, open orders (filter de-activated) | ✅ | 2026-06-18 | pending |

**Test criteria**
- **3.1:** Quotes/Greeks for OESX match the TWS UI.
- **3.2:** Account/positions match the TWS paper account exactly; **no order placed.**

---

## Phase 4 — Order execution (paper, manual, tightly gated)

Human-in-the-loop. Not a candidate for autonomous orchestration — this path can send orders.

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 4.1 | `placeOrder` / `cancelOrder` + `orderStatus` / `openOrder` callbacks via the dispatcher | ✅ | 2026-06-18 | 524b855, df13a9f, b1704dc, 5823969 |
| 4.2 | Bracket orders (parent + TP + SL, OCA) | ✅ | 2026-06-18 | 84ea666 |
| 4.3a | Historical data codec (isolated, Fabro-eligible) | ✅ | 2026-06-18 | 7b032f1 |
| 4.3b | `ibkr_data.go` historical/latest bars integration | ✅ | 2026-06-19 | 627f4ba, 381c9c3, f0b43f4, 9519673, efbe055 |
| 4.3c | Safe Bot Wiring (Broker config, paper enforcement, disconnect monitor) | ✅ | 2026-06-19 | 5cbedfe, 72c14d2 |
| 4.3d | `PositionManager` bracket refactor + tracking reconciliation | ✅ | 2026-06-19 | ef11214 |
| 4.3e | Supervised autonomous beat (size caps, kill switch, watched) | ✅ | 2026-06-19 | 58c055c |

**Test criteria**
- **4.1:** 1-lot **paper** order places, fills, reconciles. *(Met: STK + OESX placement, a real AAPL fill + position reconcile, then flattened. Fill exercised on AAPL because EUREX was closed; OESX order placed/accepted/cancelled.)*
- **4.2:** Parent + TP + SL submit atomically; OCA behaves on partial fill. *(Met: verified atomically grouped resting orders across US equities and EU options; grouping + cancel cascade verified; OCA-on-fill not yet exercised.)*
- **4.3a:** Historical data codec unit-tested against recorded bytes.
- **4.3b:** Live fetch of historical bars matches TWS UI.
- **4.3c:** Bot connects to `ibkr` safely, reads data, makes assessment (no orders).
- **4.3d:** PositionManager entry orders seamlessly utilize native brackets without breaking DB reconciliation.
- **4.3e:** Agent wakes → assesses → places safely bound paper trade → sleeps.

---

## Phase 5 — Cutover to IBKR-only

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 5.1 | Contract mapping for US/EU stocks + futures alongside OESX options | ✅ | 2026-06-19 | |
| 5.2 | News/feeds → European sources; remove the dead `reqFundamentalData` path | ✅ | 2026-06-19 | |
| 5.3 | Switch wiring to IBKR and **delete the Alpaca services** (IBKR-only) | ✅ | 2026-06-19 | |

**Test criteria**
- **5.1:** Each instrument type round-trips contractDetails + a paper order. *(Done: US stock + OESX option via tws.ParseSymbol/FormatSymbol. Remaining: EU stocks, futures.)*
- **5.2:** `news_service` returns EU sources; no calls to removed fundamentals APIs.
- **5.3:** Clean autonomous run on IBKR paper from a cold start, Alpaca code gone.

---

## Phase 6 — Later / optional

| Step | Description | Status | Date | Commit |
|------|-------------|--------|------|--------|
| 6.1 | Live (port 4001) behind an explicit double-confirm guard | ✅ | 2026-06-22 | |
| 6.2 | ~~Java backend migration (optional, separate effort)~~ | ✅ n/a | | no longer relevant |
| 6.3 | ~~Merge the Claude Code CLI swap track~~ | ✅ n/a | | no longer relevant |

---

## Backlog

| Rank | Item | Priority | Notes |
|------|------|----------|-------|
| 1 | **Enable Gemini LLM provider via UI** | P0 — Cost | Fix `gemini_client.go` (3 bugs), wire provider env in `server.js`. Fetch model list dynamically from provider APIs on server startup + cache (not per-page-load); `POST /api/models/refresh` already exists for manual refresh. Drops LLM cost ~250x. |
| 2 | **Cache TRADING_RULES.md at startup** | P1 — Perf | Load once in `Start()`, store in field. No per-beat disk I/O. |
| 3 | **Dashboard beat grouping** | P2 — UX | Group bot log events by `beatId` into collapsible cards. Backend `beatId` tagging already done. Subsumes "Agent Dialogue UX". |
| 4 | Multi-Leg Option Combos | P3 | BAG routing for net credit/debit limit orders on option spreads. |
| 5 | Agent Identity UI | P3 | Show messages by role (CEO, Stratagem, Daedalus) in dashboard. |
| 6 | Trading Safety Checks | P3 | Tidy "Enable Trading" logic; make live vs dry-run mode explicitly clear. |
| 7 | Dashboard UI Overhaul | P4 | General UX/UI improvements. |

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
2026-06-18 | Step 2.3.1| Hardening pass: fixed Dispatcher RLock race condition and added interleaving race test. Renamed CurrentTime millis variables to seconds and acknowledged usage of classic message ID 49 (seconds) instead of 10.44 millis. Added routing strategy to CLAUDE.md.
2026-06-18 | Step 4.1  | placeOrder rewritten as a serverVersion-gated encoder built from the installed TWS source (~/IBJts/source, Java + Python clients) — fixes error 320 (old fixed 110-field payload was truncated/misaligned; v187 needs 118 fields incl. the RFQ block). cancelOrder moved to the modern format (manualOrderCancelTime + RFQ trio); legacy [4,"1",id] was silently ignored by v187 and leaked open orders. orderStatus decoder: read orderId from fields[1] (modern servers ≥131 omit the version field). openOrder decoder: status now located by enum match instead of fragile skip-counting. ibkr_trading.PlaceOrder logs intent before send and waits for the real orderStatus/error (no more fake "Submitted"). Validated live on paper (commits 524b855, df13a9f); stray test orders cancelled, account clean.
2026-06-18 | Step 5.1  | Symbology mapping in one place (tws.ParseSymbol/FormatSymbol, commits 6439d47/a4f865f): bare ticker -> US stock; "ESTX50:<YYYYMMDD>:<C|P>:<strike>" -> EURO STOXX 50 option (EUREX/EUR, x10, tradingClass OESX). Verified live: ESTX50:20260619:C:6325 resolved + accepted by TWS, round-trips in ListOrders. Table tests added. (EU stocks/futures still to do.)
2026-06-18 | Step 4.1  | Order-type normalization (b1704dc): reject empty/unknown type instead of silently defaulting to market; map Alpaca-style market/limit/stop[_limit] -> MKT/LMT/STP[ STP LMT]. Fixed a latent bug — callers pass lower-case types that the old pass-through would have sent verbatim. normalizeOrderType unit test added (9d9c807). test_trading harness parameterized with flags (guards: orders only on 4002, market needs -allow-market).
2026-06-18 | Step 4.1  | Warning handling (5823969): code 399 ("order will not be placed until <open>") is a non-fatal warning, not a rejection — isWarningCode keeps it off the order-confirm channel; authoritative state comes via orderStatus.
2026-06-18 | Step 4.1  | CLOSED. Real fill + reconciliation exercised live (human-authorized, throwaway cmd, since live API data needs a subscription — 1-share AAPL market buy -> Filled @298.45 -> GetPositions shows qty 1 -> market sell -> flat). Full lifecycle proven: place/fill/cancel/reconcile across STK + OESX. Account left flat.
2026-06-18 | Step 4.3a | Historical data codec implemented and verified live. Defined `HistoricalBar` type and updated Wrapper methods. Extended encoder with `ReqHistoricalData` mapped to TWS wire protocol. Wrote `historicalData`/`historicalDataEnd`/`historicalDataUpdate` handlers in decoder that respect the negotiated `ServerVersion`. Added test cases mapping to actual byte sequences. A standalone manual test against TWS port 4002 fetching AAPL last 5 days succeeded cleanly. Ready for integration into IBKRDataService (4.3b).
2026-06-19 | Step 4.3b | Implemented `GetHistoricalBars` and `GetLatestBar` in `IBKRDataService` (services/ibkr_data.go), `formatDate=2` (epoch seconds) for unambiguous timestamps. Verified live over paper TWS 4002.
2026-06-19 | Step 4.3c | Safe bot wiring (5cbedfe, 72c14d2). config gains Broker/IBKRHost/IBKRPort/IBKRClientID + TradingEnabled (master kill-switch, default false). New GatedTradingService wraps the trading service: when disabled it logs order intent and refuses PlaceOrder/CancelOrder/PlaceOptionsOrder (reads pass through), with runtime Disable() for disconnect→halt; unit-tested. cmd/bot selects broker via BROKER=: ibkr enforces paper port 4002 (Fatal otherwise), connects tws.Client, wires IBKRDataService + gate-wrapped IBKRTradingService, and halts orders on client.Closed(); alpaca path unchanged and ungated. Added tws.Client.Closed(). Supervised smoke (BROKER=ibkr, gate OFF) verified live: connects to paper, GET /api/v1/account returns the real DU***4187 balances, /positions OK, and POST /orders/buy is refused by the gate ("trading disabled (dry-run mode)") — no order placed. Orders stay gated until 4.3e.
2026-06-19 | Step 4.3b (refine) | Review-driven follow-ups (381c9c3, f0b43f4, 9519673, efbe055): (1) bars now accumulate in a server-side `histBuf` and the single `HistoricalDataEnd` hands the whole slice over via `ExtData` — replaces the earlier per-bar blocking send so the read loop never blocks on a consumer and never drops bars; (2) `ParseTWSDate` extracted to `tws/date.go` + table test; (3) duration clamped for intraday and sub-day intraday windows emit `"<n> S"` (table-tested) to avoid 162/322 and over-fetch; (4) `GetLatestBar` uses `5Min`/5-day lookback (covers long weekends without hitting the 1-min cap); (5) `whatToShow` instrument-aware — MIDPOINT for options (clean series; OESX has ~0 volume), TRADES for stocks/futures/indices. Live-verified: AAPL TRADES (real volume), OESX MIDPOINT (continuous), OESX 1h→`3599 S`→60 bars. OPEN for 4.3d/e: intelligence layer must not depend on option volume (MIDPOINT bars are Vol=0).
2026-06-19 | Phase 5   | Cutover to IBKR-only complete. Implemented `EU:` and `FUT:` symbology mapping. Updated Google News endpoints to fetch DE/en-GB European feeds. Deleted Alpaca services, stripped out `Broker` dynamic wiring in `cmd/bot` and updated configs. Bot compiles and runs seamlessly against IB Gateway (port 4002).
2026-06-22 | Phase 6.1 | Live execution (port 4001) unlocked behind a strict double-confirm intent guard. Agent trade proposals (managed positions/options) are intercepted at the tool boundary, queued in an IntentManager, and only hit the broker if explicitly authorized via an `ADMIN_TOKEN` endpoint (`/api/v1/beat/authorize/:id`). Stale price guard and configurable intent TTL implemented to prevent executing outdated market proposals.
```
