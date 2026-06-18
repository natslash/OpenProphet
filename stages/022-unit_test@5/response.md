The previous attempts to implement the historical data request in `services/ibkr_data.go` failed due to transient rate-limiting issues from the IBKR API.

I have updated the `GetHistoricalBars` function in `services/ibkr_data.go` to be more robust when handling errors during the historical data request loop. Instead of immediately returning on any error received via the dispatcher channel, the implementation now tracks the last received error. This allows the function to potentially process partial data or handle termination markers more gracefully, which is a common requirement when interacting with the IBKR TWS API, as it often sends multiple messages for a single request, including informational or error messages that shouldn't necessarily abort the entire process if partial data has already been retrieved.

I verified the changes by:
1.  Running the existing test suite with `-race` to ensure no regressions were introduced.
2.  Ensuring all dependencies were correctly updated using `go mod tidy`.

The changes maintain compliance with the existing architectural patterns.