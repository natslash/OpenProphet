Now I have full confirmation. Here's the diagnosis.

## What went wrong

The `implement` step keeps failing, but **the failure is not in the target file `services/ibkr_data.go`** — that file compiles, vets, and passes its tests cleanly on its own. The whole-module build (`go build ./...`) is what's red, and it breaks in two unrelated, stale helper programs under `cmd/`:

**`cmd/testmktdata/main.go`**
- Line 24: `client.Connect()` — the current signature is `Connect(ctx context.Context)` (tws_client.go:82). Missing arg.
- Line 37: `Multiplier: 10` — `Contract.Multiplier` is now a `string` (contract.go:19), not an int.

**`cmd/tws_smoke_test/main.go`**
- Line 39: `client.SetWrapper(wrapper)` — no such method exists. The `Client` *is* its own wrapper now (it implements `NextValidId`, `TickPrice`, etc. directly — tws_client.go:188–428), so `SetWrapper` was removed when the design changed.

Both files were written against an **older `tws.Client` API** during earlier Phase 2 work and never updated when the client was refactored (handshake now takes a context, the wrapper got folded into `Client`, `Multiplier` became a string for the OESX `"10"` convention). They are throwaway manual smoke-test scratch programs, not part of the Phase 3.1 deliverable.

The result is a trap: the AI keeps re-touching `ibkr_data.go` (which is already correct), the unit_test stage passes (it only exercises the codec/services tests), but the gate that runs `go build ./...` stays red because of these two leftover files — so `implement` is marked failed every cycle. The previous attempts misattributed the failure to the target file instead of the stale `cmd/` programs.

## The different approach for the next attempt

Stop editing `services/ibkr_data.go` (it's done) and instead **fix the two stale `cmd/` programs** so the whole module builds:

1. **`cmd/testmktdata/main.go`** — pass a context to `Connect` (`ctx, cancel := context.WithTimeout(...)`; `client.Connect(ctx)`), and change `Multiplier: 10` → `Multiplier: "10"` to match the `string` field.
2. **`cmd/tws_smoke_test/main.go`** — remove the `client.SetWrapper(wrapper)` call (and the now-unused `smokeTestWrapper` type / `timeReceived` field), since `Client` is its own wrapper. The smoke test can just connect and rely on the client's built-in callbacks.
3. Verify with `go build ./...` **and** `go vet ./...` (not just `./services/...`), then `go test ./...`, and only mark green when the *whole module* is clean.

This stays within the Phase 3.1 scope — these are non-order, non-secret, local scratch tools, and the change is purely making them compile against the current API. No interface changes, no Alpaca deletion, no live-port code.

Would you like me to go ahead and apply these fixes to the two `cmd/` files now?