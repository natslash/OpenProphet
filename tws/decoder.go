package tws

import (
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
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

	case inTickPrice:
		d.handleTickPrice(fields)

	case inTickSize:
		d.handleTickSize(fields)

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

	case inAccountSummary:
		if len(fields) >= 7 {
			reqId, _ := strconv.ParseInt(fields[2], 10, 64)
			d.wrapper.AccountSummary(reqId, fields[3], fields[4], fields[5], fields[6])
		}

	case inAccountSummaryEnd:
		if len(fields) >= 3 {
			reqId, _ := strconv.ParseInt(fields[2], 10, 64)
			d.wrapper.AccountSummaryEnd(reqId)
		}

	case inPosition:
		if len(fields) >= 16 {
			account := fields[2]
			var c Contract
			c.ConId, _ = strconv.ParseInt(fields[3], 10, 64)
			c.Symbol = fields[4]
			c.SecType = InstrumentType(fields[5])
			c.LastTradeDateOrContractMonth = fields[6]
			c.Strike, _ = strconv.ParseFloat(fields[7], 64)
			c.Right = fields[8]
			c.Multiplier = fields[9]
			c.Exchange = fields[10]
			c.Currency = fields[11]
			c.LocalSymbol = fields[12]
			c.TradingClass = fields[13]
			
			pos, _ := decimal.NewFromString(fields[14])
			avgCost, _ := strconv.ParseFloat(fields[15], 64)
			
			d.wrapper.Position(account, c, pos, avgCost)
		}

	case inPositionEnd:
		d.wrapper.PositionEnd()

	case inOpenOrder:
		// Safe sequential parsing for OpenOrder
		orderId, c, o, os, ok := decodeOpenOrder(fields)
		if ok {
			d.wrapper.OpenOrder(orderId, c, o, os)
		}

	case inOpenOrderEnd:
		d.wrapper.OpenOrderEnd()

	case inOrderStatus:
		// Modern servers (>= MIN_SERVER_VER_MARKET_CAP_PRICE, 131) omit the
		// version field, so orderId is at index 1:
		// [3, orderId, status, filled, remaining, avgFillPrice, permId,
		//  parentId, lastFillPrice, clientId, whyHeld, mktCapPrice]
		if len(fields) >= 12 {
			orderId, _ := strconv.ParseInt(fields[1], 10, 64)
			status := fields[2]
			filled, _ := decimal.NewFromString(fields[3])
			remaining, _ := decimal.NewFromString(fields[4])
			avgFillPrice, _ := strconv.ParseFloat(fields[5], 64)
			permId, _ := strconv.ParseInt(fields[6], 10, 64)
			parentId, _ := strconv.ParseInt(fields[7], 10, 64)
			lastFillPrice, _ := strconv.ParseFloat(fields[8], 64)
			clientId, _ := strconv.Atoi(fields[9])
			whyHeld := fields[10]
			mktCapPrice, _ := strconv.ParseFloat(fields[11], 64)
			d.wrapper.OrderStatus(orderId, status, filled, remaining, avgFillPrice, permId, parentId, lastFillPrice, clientId, whyHeld, mktCapPrice)
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

func (d *Decoder) handleTickPrice(fields []string) {
	if len(fields) < 6 {
		return
	}
	version, _ := strconv.Atoi(fields[1])
	reqId, _ := strconv.ParseInt(fields[2], 10, 64)
	tickType, _ := strconv.Atoi(fields[3])
	price, _ := strconv.ParseFloat(fields[4], 64)

	size := decimal.Zero
	if version >= 2 {
		size, _ = decimal.NewFromString(fields[5])
	}

	attr := TickAttrib{}
	if version >= 3 && len(fields) >= 7 {
		attrMask, _ := strconv.Atoi(fields[6])
		attr.CanAutoExecute = (attrMask & 1) != 0
		if version >= 4 {
			attr.PastLimit = (attrMask & 2) != 0
		}
		if version >= 6 {
			attr.PreOpen = (attrMask & 4) != 0
		}
	}

	d.wrapper.TickPrice(reqId, tickType, price, size, attr)
}

func (d *Decoder) handleTickSize(fields []string) {
	if len(fields) < 5 {
		return
	}
	reqId, _ := strconv.ParseInt(fields[2], 10, 64)
	tickType, _ := strconv.Atoi(fields[3])
	size, _ := decimal.NewFromString(fields[4])

	d.wrapper.TickSize(reqId, tickType, size)
}
