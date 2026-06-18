Confirmed. Here's the complete diagnosis.

## What went wrong

The `implement` stage keeps failing because **`go build ./...` does not compile** — but the failure is *not* in the file being worked on. `services/ibkr_data.go` (the Phase 3.1 target) compiles fine, and the unit tests in `tws/` pass. The build breaks in two stale helper commands that were left behind when the `tws.Client` API evolved during Phase 2:

1. **`cmd/testmktdata/main.go`**
   - Line 24: calls `client.Connect()` with no arguments, but the API is now `Connect(ctx context.Context)`.
   - Line 37: sets `Multiplier: 10` (an int) on `tws.Contract`, but `Contract.Multiplier` is a `string`.

2. **`cmd/tws_smoke_test/main.go`**
   - Line 39: calls `client.SetWrapper(wrapper)`, but `Client` has no such method. The client *is* its own `Wrapper` now (it implements `NextValidId`, `TickPrice`, etc. directly — see `tws_client.go:188–428`), so external wrapper injection was removed.

Because `go build ./...` and `go test ./...` walk every package, these two unmaintained `cmd/` programs fail the build gate even though the actual feature work is sound. That's why `unit_test` (which presumably runs only `tws/` tests) succeeds while `implement` (which must produce a green full build) fails — and it has failed identically three times because nothing addressed these two files.

## Why it kept repeating

Each retry re-implemented or re-touched `services/ibkr_data.go` and `tws/` test/wrapper files (per the stage file lists), but never touched `cmd/testmktdata` or `cmd/tws_smoke_test`. The root cause sat outside the files being edited, so the build kept failing at the same two locations.

## Different approach for the next attempt

Fix the build by updating the two stale helper commands to the current `tws.Client` API — a minimal, mechanical change, not a redesign:

1. **`cmd/testmktdata/main.go`**
   - Add a `context` import and pass a context to `Connect`: `client.Connect(ctx)` with a `context.WithTimeout`.
   - Change `Multiplier: 10` → `Multiplier: "10"` to match the `string` field type.

2. **`cmd/tws_smoke_test/main.go`**
   - Remove the `client.SetWrapper(wrapper)` call and the now-unused `smokeTestWrapper` type, since `Client` already routes callbacks internally. (These are debug-only smoke programs, so the printing wrapper isn't needed for the build to pass.)

3. **Verify with the same gate the stage uses:** run `go build ./...` and `go test ./...` to confirm the whole module is green before claiming the step done — not just the `tws/` package.

This targets the actual root cause (stale `cmd/` helpers against the evolved client API) instead of re-editing the already-correct Phase 3.1 file.

Want me to apply these fixes now, or only update `IBKR_MIGRATION_PLAN_v2.md`/`PROGRESS.md` context first?