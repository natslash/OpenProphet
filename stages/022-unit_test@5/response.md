The implementation of Phase 3.2, `services/ibkr_trading.go` read-only paths (account, positions), is complete.

*   `GetAccount` implemented via `ReqAccountSummary` and mapped to `interfaces.Account`.
*   `GetPositions` implemented via `ReqPositions` and mapped to `interfaces.Position`.
*   Trading/Options methods are stubbed out as requested to satisfy the `interfaces.TradingService` interface, throwing "not implemented" errors.
*   Unit tests in `services/ibkr_trading_test.go` pass.
*   `go test ./services/...` passed with `-race`.

**Next action:** Verify these read paths live against the IB Gateway paper account (Phase 3.2 test criterion).

```bash
# To run the tests again:
export PATH=$PATH:/usr/local/go/bin
go test -race -v ./services/
```