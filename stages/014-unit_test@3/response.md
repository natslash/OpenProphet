The goal of implementing `GetHistoricalBars` for `IbkrDataService` has been achieved.

Here is a summary of the changes:
1.  **`tws/constants.go`**: Added `outReqHistoricalData` and `inHistoricalData` constants to support the historical data request and response protocol.
2.  **`tws/encoder.go`**: Implemented `ReqHistoricalData` in the `Encoder` to properly format and send historical data requests to the IBKR TWS API.
3.  **`tws/wrapper.go`**: Updated the `Wrapper` interface to include the `HistoricalData` callback method and added the `HistoricalData` struct to define the structure of incoming bar data.
4.  **`tws/decoder.go`**: Added `handleHistoricalData` to parse the incoming historical data responses and dispatch them through the `Wrapper`.
5.  **`tws/tws_client.go`**: Added the implementation of the `HistoricalData` method to the `Client` struct to fulfill the updated `Wrapper` interface and ensure the data is dispatched to the registered handlers.
6.  **`services/ibkr_data.go`**: Implemented the `GetHistoricalBars` method. It resolves the symbol to an IBKR `Contract`, uses the new `ReqHistoricalData` encoder method to request the data, and collects the responses into a slice of `interfaces.Bar`.

All tests passed with `go test -race ./tws/... ./services/...`.