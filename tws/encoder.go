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
