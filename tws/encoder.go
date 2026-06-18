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
func (e *Encoder) PlaceOrder(reqId int64, contract Contract, order Order) error {
	// Send place order message. v100+ servers use a more complex format, but the essential 
	// fields can be sent in version 45 logic. Since TWS maintains backward compatibility,
	// we will send version 45 which is sufficient for basic orders.
	const version = "45"
	
	strikeStr := strconv.FormatFloat(contract.Strike, 'f', -1, 64)
	if contract.Strike == 0.0 {
		strikeStr = "0"
	}

	fields := []string{
		strconv.Itoa(outPlaceOrder),
		version,
		strconv.FormatInt(reqId, 10),
		
		// Contract fields
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
		"0", // secIdType
		"",  // secId

		// Order fields
		order.Action,
		order.TotalQuantity.String(),
		order.OrderType,
		strconv.FormatFloat(order.LmtPrice, 'f', -1, 64),
		strconv.FormatFloat(order.AuxPrice, 'f', -1, 64),
		order.Tif,
		order.OcaGroup,
		order.Account,
		"", // openClose
		"", // origin
		"", // orderRef
		"1", // transmit
		"0", // parentId
		"0", // blockOrder
		"0", // sweepToFill
		"0", // displaySize
		"0", // triggerMethod
		"0", // outsideRth
		"0", // hidden
		"",  // goodAfterTime
		"",  // goodTillDate
		"",  // rule80A
		"0", // percentOffset
		"0", // trailingPercent
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
		"", // manualOrderCancelTime (empty for automated)
	}
	return e.writer.SendFields(fields...)
}
