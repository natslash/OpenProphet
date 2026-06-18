package tws

import "strconv"

// FieldWriter represents a transport that can send length-prefixed,
// null-delimited TWS messages.
type FieldWriter interface {
	SendFields(fields ...string) error
}

// Encoder translates typed request methods into TWS wire format messages.
type Encoder struct {
	writer FieldWriter
}

// NewEncoder creates a new protocol encoder.
func NewEncoder(w FieldWriter) *Encoder {
	return &Encoder{writer: w}
}

// ReqCurrentTime requests the current system time on the server side.
// The TWS server will respond with an inCurrentTime message containing the epoch time.
func (e *Encoder) ReqCurrentTime() error {
	const version = "1"
	return e.writer.SendFields(strconv.Itoa(outReqCurrentTime), version)
}

// ReqContractDetails requests contract details.
func (e *Encoder) ReqContractDetails(reqId int64, contract Contract) error {
	const version = "8"

	strikeStr := strconv.FormatFloat(contract.Strike, 'f', -1, 64)
	if contract.Strike == 0.0 {
		strikeStr = "0.0"
	}

	fields := []string{
		strconv.Itoa(outReqContractDetails),
		version,
		strconv.FormatInt(reqId, 10),
		strconv.FormatInt(contract.ConId, 10),
		contract.Symbol,
		string(contract.SecType),
		contract.LastTradeDateOrContractMonth,
		strikeStr,
		contract.Right,
		contract.Multiplier,
		contract.Exchange,
		contract.PrimaryExch,
		contract.Currency,
		contract.LocalSymbol,
		contract.TradingClass,
		"0", // includeExpired
		"",  // secIdType
		"",  // secId
		"",  // issuerId
	}

	return e.writer.SendFields(fields...)
}

// ReqMktData requests market data.
func (e *Encoder) ReqMktData(reqId int64, contract Contract, genericTickList string, snapshot bool, regulatorySnapshot bool) error {
	const version = "11"

	strikeStr := strconv.FormatFloat(contract.Strike, 'f', -1, 64)
	if contract.Strike == 0.0 {
		strikeStr = "0.0"
	}

	snapStr := "0"
	if snapshot {
		snapStr = "1"
	}
	regSnapStr := "0"
	if regulatorySnapshot {
		regSnapStr = "1"
	}

	fields := []string{
		strconv.Itoa(outReqMktData),
		version,
		strconv.FormatInt(reqId, 10),
		strconv.FormatInt(contract.ConId, 10),
		contract.Symbol,
		string(contract.SecType),
		contract.LastTradeDateOrContractMonth,
		strikeStr,
		contract.Right,
		contract.Multiplier,
		contract.Exchange,
		contract.PrimaryExch,
		contract.Currency,
		contract.LocalSymbol,
		contract.TradingClass,
		"0", // underComp
		genericTickList,
		snapStr,
		regSnapStr,
		"", // mktDataOptions
	}

	return e.writer.SendFields(fields...)
}

// CancelMktData cancels a market data request.
func (e *Encoder) CancelMktData(reqId int64) error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outCancelMktData),
		version,
		strconv.FormatInt(reqId, 10),
	}
	return e.writer.SendFields(fields...)
}

// ReqAccountSummary requests account summary data.
func (e *Encoder) ReqAccountSummary(reqId int64, group, tags string) error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outReqAccountSummary),
		version,
		strconv.FormatInt(reqId, 10),
		group,
		tags,
	}
	return e.writer.SendFields(fields...)
}

// CancelAccountSummary cancels an account summary request.
func (e *Encoder) CancelAccountSummary(reqId int64) error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outCancelAccountSummary),
		version,
		strconv.FormatInt(reqId, 10),
	}
	return e.writer.SendFields(fields...)
}

// ReqPositions requests all positions for all accounts.
// TWS API doesn't use a reqId for positions, so we just send the message.
func (e *Encoder) ReqPositions() error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outReqPositions),
		version,
	}
	return e.writer.SendFields(fields...)
}

// ReqOpenOrders requests all open orders.
func (e *Encoder) ReqOpenOrders() error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outReqOpenOrders),
		version,
	}
	return e.writer.SendFields(fields...)
}

// PlaceOrder places a new order or updates an existing order.

func formatPrice(p float64) string {
	if p == 0.0 { // TWS usually treats 0.0 or 1.79e308 as unset. We'll use empty string for unset.
		return ""
	}
	return strconv.FormatFloat(p, 'f', -1, 64)
}

func (e *Encoder) PlaceOrder(reqId int64, contract Contract, order Order) error {
	// Send place order message. v145+ servers do NOT expect the version string.
	// Since we assume TWS v187, we omit the version field entirely.

	strikeStr := strconv.FormatFloat(contract.Strike, 'f', -1, 64)
	if contract.Strike == 0.0 {
		strikeStr = "0"
	}

	fields := []string{
		strconv.Itoa(outPlaceOrder),           // 0
		strconv.FormatInt(reqId, 10),          // 1
		strconv.FormatInt(contract.ConId, 10), // 2
		contract.Symbol,                       // 3
		string(contract.SecType),              // 4
		contract.LastTradeDateOrContractMonth, // 5
		strikeStr,                             // 6
		contract.Right,                        // 7
		contract.Multiplier,                   // 8
		contract.Exchange,                     // 9
		contract.PrimaryExch,                  // 10
		contract.Currency,                     // 11
		contract.LocalSymbol,                  // 12
		contract.TradingClass,                 // 13
		"",                                    // secIdType // 14
		"",                                    // secId // 15
		order.Action,                          // 16
		order.TotalQuantity.String(),          // 17
		order.OrderType,                       // 18
		formatPrice(order.LmtPrice),           // 19
		formatPrice(order.AuxPrice),           // 20
		order.Tif,                             // 21
		order.OcaGroup,                        // 22
		order.Account,                         // 23
		"",                                    // 24
		"0",                                   // 25
		"",                                    // 26
		"1",                                   // 27
		"0",                                   // 28
		"0",                                   // 29
		"0",                                   // 30
		"0",                                   // 31
		"0",                                   // 32
		"0",                                   // 33
		"0",                                   // 34
		"",                                    // 35
		"0",                                   // 36
		"",                                    // 37
		"",                                    // 38
		"",                                    // 39
		"",                                    // 40
		"",                                    // 41
		"",                                    // 42
		"",                                    // 43
		"0",                                   // 44
		"",                                    // 45
		"-1",                                  // 46
		"0",                                   // 47
		"",                                    // 48
		"",                                    // 49
		"0",                                   // 50
		"",                                    // 51
		"",                                    // 52
		"1",                                   // 53
		"1",                                   // 54
		"",                                    // 55
		"0",                                   // 56
		"",                                    // 57
		"",                                    // 58
		"",                                    // 59
		"",                                    // 60
		"",                                    // 61
		"0",                                   // 62
		"",                                    // 63
		"",                                    // 64
		"",                                    // 65
		"",                                    // 66
		"0",                                   // 67
		"",                                    // 68
		"",                                    // 69
		"",                                    // 70
		"",                                    // 71
		"",                                    // 72
		"",                                    // 73
		"",                                    // 74
		"",                                    // 75
		"",                                    // 76
		"",                                    // 77
		"0",                                   // 78
		"",                                    // 79
		"",                                    // 80
		"0",                                   // 81
		"0",                                   // 82
		"",                                    // 83
		"",                                    // 84
		"0",                                   // 85
		"",                                    // 86
		"0",                                   // 87
		"0",                                   // 88
		"0",                                   // 89
		"0",                                   // 90
		"",                                    // 91
		"1.7976931348623157e+308",             // 92
		"1.7976931348623157e+308",             // 93
		"1.7976931348623157e+308",             // 94
		"1.7976931348623157e+308",             // 95
		"1.7976931348623157e+308",             // 96
		"0",                                   // 97
		"",                                    // 98
		"",                                    // 99
		"",                                    // 100
		"1.7976931348623157e+308",             // 101
		"",                                    // 102
		"",                                    // 103
		"",                                    // 104
		"",                                    // 105
		"0",                                   // 106
		"0",                                   // 107
		"0",                                   // 108
		"",                                    // 109
		"",                                    // 110
	}

	return e.writer.SendFields(fields...)
}

// CancelOrder cancels an active order.
func (e *Encoder) CancelOrder(reqId int64) error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outCancelOrder),
		version,
		strconv.FormatInt(reqId, 10),
	}
	return e.writer.SendFields(fields...)
}
