The implementation of `GetLatestQuote` in `services/ibkr_data.go` has been successfully completed, enabling the retrieval of real-time market quotes using the IBKR TWS API client.

Key changes:
1.  **`services/ibkr_data.go`**: Implemented `GetLatestQuote` which:
    *   Resolves a symbol to a TWS `Contract`.
    *   Registers a unique request ID with the TWS `Dispatcher`.
    *   Requests market data from TWS using `ReqMktData`.
    *   Waits for and collects `TickPrice` and `TickSize` messages to form an `interfaces.Quote`.
2.  **`tws/tws_client.go`**: Added the `Dispatcher()` accessor method to the `Client` struct to allow services to access the dispatcher for request registration and tracking.

Validation:
*   Ran all tests via `go test -race ./tws/... ./services/...` and confirmed they pass.
*   Verified the new logic compiles correctly.