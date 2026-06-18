package tws

import (
	"fmt"
	"strconv"
)

// Decoder parses incoming \0-delimited TWS messages and dispatches them
// to the appropriate methods on the Wrapper interface.
type Decoder struct {
	wrapper Wrapper
}

// NewDecoder creates a new protocol decoder.
func NewDecoder(w Wrapper) *Decoder {
	return &Decoder{wrapper: w}
}

// Decode processes a single incoming TWS message payload.
func (d *Decoder) Decode(fields []string) error {
	if len(fields) == 0 || fields[0] == "" {
		return nil
	}

	msgID, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("invalid message ID %q: %w", fields[0], err)
	}

	switch msgID {
	case inNextValidID: // [9, version, orderId]
		if len(fields) >= 3 {
			if orderID, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.NextValidId(orderID)
			}
		}

	case inManagedAccts: // [15, version, accountsCSV]
		if len(fields) >= 3 {
			d.wrapper.ManagedAccounts(fields[2])
		}

	case inErrMsg: // [4, version, id, code, msg, ...]
		if len(fields) >= 5 {
			reqID, _ := strconv.Atoi(fields[2])
			code, _ := strconv.Atoi(fields[3])
			d.wrapper.Error(reqID, code, fields[4])
		}

	case inCurrentTime: // [49, version, time]
		if len(fields) >= 3 {
			if t, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.CurrentTime(t)
			}
		}

	case inContractData:
		d.handleContractData(fields)

	case inContractDataEnd: // [52, version, reqId]
		if len(fields) >= 3 {
			if reqId, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.ContractDetailsEnd(reqId)
			}
		}

	default:
		// Unhandled message type in this phase
	}

	return nil
}

func (d *Decoder) handleContractData(fields []string) {
	if len(fields) < 30 {
		return
	}
	
	reqId, _ := strconv.ParseInt(fields[2], 10, 64)
	
	var cd ContractDetails
	cd.Contract.Symbol = fields[3]
	cd.Contract.SecType = InstrumentType(fields[4])
	cd.Contract.LastTradeDateOrContractMonth = fields[5]
	cd.Contract.Strike, _ = strconv.ParseFloat(fields[6], 64)
	cd.Contract.Right = fields[7]
	cd.Contract.Exchange = fields[8]
	cd.Contract.Currency = fields[9]
	cd.Contract.LocalSymbol = fields[10]
	cd.MarketName = fields[11]
	cd.Contract.TradingClass = fields[12]
	cd.Contract.ConId, _ = strconv.ParseInt(fields[13], 10, 64)
	cd.MinTick, _ = strconv.ParseFloat(fields[14], 64)
	cd.MdSizeMultiplier, _ = strconv.ParseInt(fields[15], 10, 64)
	cd.Contract.Multiplier = fields[16]
	cd.OrderTypes = fields[17]
	cd.ValidExchanges = fields[18]
	cd.PriceMagnifier, _ = strconv.ParseInt(fields[19], 10, 64)
	cd.UnderConId, _ = strconv.ParseInt(fields[20], 10, 64)
	cd.LongName = fields[21]
	cd.Contract.PrimaryExch = fields[22]
	cd.ContractMonth = fields[23]
	cd.Industry = fields[24]
	cd.Category = fields[25]
	cd.Subcategory = fields[26]
	cd.TimeZoneId = fields[27]
	cd.TradingHours = fields[28]
	cd.LiquidHours = fields[29]
	if len(fields) >= 32 {
		cd.EvRule = fields[30]
		cd.EvMultiplier, _ = strconv.ParseFloat(fields[31], 64)
	}

	d.wrapper.ContractDetails(reqId, cd)
}
