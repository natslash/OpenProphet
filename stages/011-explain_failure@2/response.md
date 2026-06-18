Confirmed. Here's the finalized plan for the next implementation attempt.

## Summary

**Root cause of the failed `implement` stage:** `go build ./...` fails — but not in the Phase 3.1 deliverable. `services/ibkr_data.go` compiles cleanly and its `interfaces.DataService` assertion holds. The breakage is in two stale Phase 2.5 manual-probe binaries that were never updated to the current `tws.Client` API:

- `cmd/testmktdata/main.go` — calls `Connect()` (now needs `ctx`); sets `Multiplier: 10` (field is `string`).
- `cmd/tws_smoke_test/main.go` — calls `client.SetWrapper(...)`, a method that no longer exists (the `Client` is its own wrapper).

Both prior attempts only looked at the named deliverable and never ran a full-tree build, so the unrelated `cmd/` breakage survived twice. `unit_test` "succeeded" because the failing packages are test-less `cmd/` binaries.

**Next attempt will:**
1. **Delete** `cmd/testmktdata/` and `cmd/tws_smoke_test/` — they were throwaway Phase 2.5 manual probes (per your choice).
2. Leave `services/ibkr_data.go` as-is (it's correct).
3. **Gate on full-tree verification:** require both `go build ./...` and `go test ./...` green across the whole repo before marking Phase 3.1 ✅ — the missing gate that let this slip.

This is ready for the retry approval gate.