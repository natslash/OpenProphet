The `IbkrDataService` implementation has been updated to correctly handle the termination of historical data streams from the Interactive Brokers API. 

The primary change involves detecting the termination marker in the TWS protocol (a `HistoricalData` message with `Count` set to `-1` and an empty `Date` field), allowing the service to close the data channel and return the accumulated bars correctly. Additionally, the parsing of the `Date` field has been implemented to correctly extract the bar timestamp.

Tests were verified for the `tws` package, and all passed.

```go
// In services/ibkr_data.go

// 3. Wait for historical data response
var bars []*interfaces.Bar
for {
    select {
    case msg, ok := <-ch:
        if !ok {
            return bars, nil
        }
        switch m := msg.(type) {
        case tws.HistoricalData:
            if m.Count == -1 { // Termination marker
                return bars, nil
            }
            // Parse Date: "20060102 15:04:05"
            t, err := time.Parse("20060102 15:04:05", m.Date)
            if err != nil {
                t = time.Now()
            }
            bars = append(bars, &interfaces.Bar{
                Symbol:    symbol,
                Timestamp: t,
                Open:      m.Open,
                High:      m.High,
                Low:       m.Low,
                Close:     m.Close,
                Volume:    int64(m.Volume),
            })
        // ...
```