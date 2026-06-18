Confirmed. Here's my analysis of what went wrong and the corrected approach.

## What went wrong

The **`implement` stage failed because it worked on the wrong thing and produced code that doesn't compile.**

**The assigned task (Phase 3.1)** was: implement `services/ibkr_data.go` so it fulfills `interfaces.DataService` over the `tws/` wrapper (quotes, bars, trades for OESX), with a compile-time assertion.

**What the failed attempt actually did:** instead of touching `ibkr_data.go`, it created a throwaway manual test harness `cmd/testmktdata/main.go`. That file has two compile errors that break `go build ./...`:

- `cmd/testmktdata/main.go:24` — calls `client.Connect()` with no args, but the real signature is `Connect(ctx context.Context)` (tws_client.go:82).
- `cmd/testmktdata/main.go:37` — sets `Multiplier: 10` (int) on a `tws.Contract` field that is a `string`.

It also hand-wrote a `dummyWrapper` whose `Error`/`ContractDetails` method signatures were guesses that don't match the real `tws.Wrapper` interface.

**Why the later stages still "succeeded":** the `unit_test` stage's tests scope `./services/...` and `./tws/...`, which pass — they never compile the broken `cmd/testmktdata` package. So the build break slipped through, and `PROGRESS.md` was marked 3.1 ✅ even though `ibkr_data.go` is still entirely stubbed (every method returns `"not implemented for IBKR yet"`).

**Net state:** Phase 3.1 is *not* actually done, and the repo doesn't build cleanly.

## What the next attempt will do differently

1. **Delete the off-task, broken file.** Remove `cmd/testmktdata/main.go` — it was never part of Phase 3.1, doesn't compile, and a manual TWS smoke test belongs in `cmd/twsconnect`/`twscheck` patterns if needed at all (and only against paper 4002).

2. **Actually implement `services/ibkr_data.go`** against the *real* `tws` signatures I just verified, rather than guessing:
   - `GetLatestQuote` / `GetLatestTrade` / `GetLatestBar`: resolve the symbol to a `tws.Contract`, call `client.Encoder().ReqMktData(...)`, and bridge the `TickPrice`/`TickSize` callbacks (note: sizes are `decimal.Decimal` per 10.44) back through a channel into the `interfaces.Quote`/`Trade`/`Bar` shapes.
   - `GetHistoricalBars`: wire `ReqHistoricalData` through the dispatcher (reqId→channel, complete on the End message).
   - Keep the `var _ interfaces.DataService = (*IbkrDataService)(nil)` assertion so an incomplete impl fails at compile time.

3. **Gate on a full build, not just package tests.** Run `go build ./...` (the whole module, including `cmd/`) and `go vet ./...` before claiming the step done — that's exactly the check that would have caught this failure.

4. **Verify the wrapper contract first.** Read `tws/wrapper.go`, `encoder.go`, and `decoder.go` for exact method signatures before writing callback-handling code, so the bridge matches the existing interface instead of a guessed one.

5. **Only mark PROGRESS 3.1 ✅ after the build is green and the assertion compiles** — and correct the premature ✅ that's already in `PROGRESS.md`.

Before I proceed with the fix, two boundary checks per the guardrails:

- Implementing `ibkr_data.go` is read-only market data (no order paths), so it's in-scope.
- Any live verification would target **paper port 4002 only**.

Want me to proceed with this corrected implementation now? I'll start by deleting the broken `cmd/testmktdata/main.go` and reading the `tws` wrapper/encoder signatures, then implement `ibkr_data.go`.