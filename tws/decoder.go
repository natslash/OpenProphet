package tws

import (
	"fmt"
	"math"
	"strconv"

	"github.com/shopspring/decimal"
)

// Decoder parses incoming \0-delimited TWS messages and dispatches them
// to the appropriate methods on the Wrapper interface.
type Decoder struct {
	wrapper       Wrapper
	serverVersion int
}

// NewDecoder creates a new protocol decoder.
func NewDecoder(w Wrapper) *Decoder {
	return &Decoder{wrapper: w}
}

// SetServerVersion updates the decoder's knowledge of the negotiated protocol version.
func (d *Decoder) SetServerVersion(v int) {
	d.serverVersion = v
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

	case inMarketDataType: // [58, version, reqId, marketDataType]
		if len(fields) >= 4 {
			reqId, _ := strconv.ParseInt(fields[2], 10, 64)
			mdt, _ := strconv.Atoi(fields[3])
			d.wrapper.MarketDataType(reqId, mdt)
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

	case inPortfolioValue:
		d.handlePortfolioValue(fields)

	case inAcctUpdateTime:
		// Ignored — we don't need the account update timestamp

	case inAcctDownloadEnd:
		if len(fields) >= 3 {
			d.wrapper.AccountDownloadEnd(fields[2])
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

	case inHistoricalData:
		d.handleHistoricalData(fields)

	case inHistoricalDataUpdate:
		d.handleHistoricalDataUpdate(fields)

	case inHistoricalDataEnd:
		d.handleHistoricalDataEnd(fields)

	case inSecDefOptParam:
		d.handleSecDefOptParam(fields)

	case inSecDefOptParamEnd:
		if len(fields) >= 2 {
			reqId, _ := strconv.ParseInt(fields[1], 10, 64)
			d.wrapper.SecurityDefinitionOptionParameterEnd(reqId)
		}

	case inTickOptionComp:
		d.handleTickOptionComputation(fields)

	default:
		// Unhandled message type in this phase
	}

	return nil
}

func (d *Decoder) handleContractData(fields []string) {
	if len(fields) < 10 {
		return
	}
	idx := 1

	// Server >= 164 (MIN_SERVER_VER_SIZE_RULES) omits the version field.
	version := 8
	if d.serverVersion < 164 {
		version, _ = strconv.Atoi(fields[idx]); idx++
	}

	var reqId int64
	if version >= 3 {
		reqId, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	}

	var cd ContractDetails
	cd.Contract.Symbol = fields[idx]; idx++
	cd.Contract.SecType = InstrumentType(fields[idx]); idx++
	cd.Contract.LastTradeDateOrContractMonth = fields[idx]; idx++
	if d.serverVersion >= 182 && idx < len(fields) {
		idx++ // lastTradeDate (separate field on modern servers)
	}
	if idx >= len(fields) { return }
	cd.Contract.Strike, _ = strconv.ParseFloat(fields[idx], 64); idx++
	if idx >= len(fields) { return }
	cd.Contract.Right = fields[idx]; idx++
	cd.Contract.Exchange = fields[idx]; idx++
	cd.Contract.Currency = fields[idx]; idx++
	cd.Contract.LocalSymbol = fields[idx]; idx++
	cd.MarketName = fields[idx]; idx++
	cd.Contract.TradingClass = fields[idx]; idx++
	cd.Contract.ConId, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	cd.MinTick, _ = strconv.ParseFloat(fields[idx], 64); idx++

	if d.serverVersion >= 106 && d.serverVersion < 164 && idx < len(fields) {
		cd.MdSizeMultiplier, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	}

	if idx >= len(fields) { return }
	cd.Contract.Multiplier = fields[idx]; idx++
	cd.OrderTypes = fields[idx]; idx++
	cd.ValidExchanges = fields[idx]; idx++

	if version >= 2 && idx < len(fields) {
		cd.PriceMagnifier, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	}
	if version >= 4 && idx < len(fields) {
		cd.UnderConId, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	}
	if version >= 5 && idx+1 < len(fields) {
		cd.LongName = fields[idx]; idx++
		cd.Contract.PrimaryExch = fields[idx]; idx++
	}
	if version >= 6 && idx+6 < len(fields) {
		cd.ContractMonth = fields[idx]; idx++
		cd.Industry = fields[idx]; idx++
		cd.Category = fields[idx]; idx++
		cd.Subcategory = fields[idx]; idx++
		cd.TimeZoneId = fields[idx]; idx++
		cd.TradingHours = fields[idx]; idx++
		cd.LiquidHours = fields[idx]; idx++
	}
	if version >= 8 && idx+1 < len(fields) {
		cd.EvRule = fields[idx]; idx++
		cd.EvMultiplier, _ = strconv.ParseFloat(fields[idx], 64); idx++
	}
	// Skip remaining modern fields (secIdList, aggGroup, underSymbol, etc.)

	_ = version
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

func (d *Decoder) handleHistoricalData(fields []string) {
	idx := 1 // fields[0] is msgID
	if idx >= len(fields) {
		return
	}

	version := math.MaxInt32
	if d.serverVersion < 32 /* MIN_SERVER_VER_SYNT_REALTIME_BARS */ {
		version, _ = strconv.Atoi(fields[idx])
		idx++
	}

	if idx >= len(fields) { return }
	reqId, _ := strconv.ParseInt(fields[idx], 10, 64)
	idx++

	startDateStr := ""
	endDateStr := ""
	if version >= 2 && d.serverVersion < 196 /* MIN_SERVER_VER_HISTORICAL_DATA_END */ {
		if idx+1 >= len(fields) { return }
		startDateStr = fields[idx]
		idx++
		endDateStr = fields[idx]
		idx++
	}

	if idx >= len(fields) { return }
	itemCount, _ := strconv.Atoi(fields[idx])
	idx++

	for ctr := 0; ctr < itemCount; ctr++ {
		if idx+8 > len(fields) { return } // At least 8 fields per bar
		date := fields[idx]
		idx++
		open, _ := strconv.ParseFloat(fields[idx], 64)
		idx++
		high, _ := strconv.ParseFloat(fields[idx], 64)
		idx++
		low, _ := strconv.ParseFloat(fields[idx], 64)
		idx++
		close, _ := strconv.ParseFloat(fields[idx], 64)
		idx++
		
		volume := decimal.Zero
		if fields[idx] != "-1" {
			volume, _ = decimal.NewFromString(fields[idx])
		}
		idx++

		wap := decimal.Zero
		if fields[idx] != "-1" {
			wap, _ = decimal.NewFromString(fields[idx])
		}
		idx++

		if d.serverVersion < 32 {
			idx++ // hasGaps
		}

		barCount := -1
		if version >= 3 {
			if idx >= len(fields) { return }
			barCount, _ = strconv.Atoi(fields[idx])
			idx++
		}

		d.wrapper.HistoricalData(reqId, HistoricalBar{
			Date:     date,
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   volume,
			BarCount: barCount,
			WAP:      wap,
		})
	}

	if d.serverVersion < 196 /* MIN_SERVER_VER_HISTORICAL_DATA_END */ {
		d.wrapper.HistoricalDataEnd(reqId, startDateStr, endDateStr)
	}
}

func (d *Decoder) handleHistoricalDataUpdate(fields []string) {
	if len(fields) < 10 {
		return
	}
	reqId, _ := strconv.ParseInt(fields[1], 10, 64)
	barCount, _ := strconv.Atoi(fields[2])
	date := fields[3]
	open, _ := strconv.ParseFloat(fields[4], 64)
	close, _ := strconv.ParseFloat(fields[5], 64)
	high, _ := strconv.ParseFloat(fields[6], 64)
	low, _ := strconv.ParseFloat(fields[7], 64)
	
	wap := decimal.Zero
	if fields[8] != "-1" {
		wap, _ = decimal.NewFromString(fields[8])
	}
	
	volume := decimal.Zero
	if fields[9] != "-1" {
		volume, _ = decimal.NewFromString(fields[9])
	}

	d.wrapper.HistoricalDataUpdate(reqId, HistoricalBar{
		Date:     date,
		Open:     open,
		High:     high,
		Low:      low,
		Close:    close,
		Volume:   volume,
		BarCount: barCount,
		WAP:      wap,
	})
}

func (d *Decoder) handleHistoricalDataEnd(fields []string) {
	if len(fields) < 4 {
		return
	}
	reqId, _ := strconv.ParseInt(fields[1], 10, 64)
	startDateStr := fields[2]
	endDateStr := fields[3]
	d.wrapper.HistoricalDataEnd(reqId, startDateStr, endDateStr)
}

func (d *Decoder) handlePortfolioValue(fields []string) {
	if len(fields) < 3 {
		return
	}
	idx := 1
	version, _ := strconv.Atoi(fields[idx]); idx++

	var c Contract
	if version >= 6 {
		c.ConId, _ = strconv.ParseInt(fields[idx], 10, 64); idx++
	}
	c.Symbol = fields[idx]; idx++
	c.SecType = InstrumentType(fields[idx]); idx++
	c.LastTradeDateOrContractMonth = fields[idx]; idx++
	c.Strike, _ = strconv.ParseFloat(fields[idx], 64); idx++
	c.Right = fields[idx]; idx++
	if version >= 7 {
		c.Multiplier = fields[idx]; idx++
		c.PrimaryExch = fields[idx]; idx++
	}
	c.Currency = fields[idx]; idx++
	if version >= 2 {
		c.LocalSymbol = fields[idx]; idx++
	}
	if version >= 8 {
		c.TradingClass = fields[idx]; idx++
	}

	if idx >= len(fields) { return }
	position, _ := decimal.NewFromString(fields[idx]); idx++
	if idx >= len(fields) { return }
	marketPrice, _ := strconv.ParseFloat(fields[idx], 64); idx++
	if idx >= len(fields) { return }
	marketValue, _ := strconv.ParseFloat(fields[idx], 64); idx++

	var averageCost, unrealizedPNL, realizedPNL float64
	if version >= 3 && idx+2 < len(fields) {
		averageCost, _ = strconv.ParseFloat(fields[idx], 64); idx++
		unrealizedPNL, _ = strconv.ParseFloat(fields[idx], 64); idx++
		realizedPNL, _ = strconv.ParseFloat(fields[idx], 64); idx++
	}

	var accountName string
	if version >= 4 && idx < len(fields) {
		accountName = fields[idx]
	}

	d.wrapper.UpdatePortfolio(c, position, marketPrice, marketValue, averageCost, unrealizedPNL, realizedPNL, accountName)
}

func (d *Decoder) handleSecDefOptParam(fields []string) {
	if len(fields) < 7 {
		return
	}
	idx := 1
	reqId, _ := strconv.ParseInt(fields[idx], 10, 64); idx++
	exchange := fields[idx]; idx++
	underlyingConId, _ := strconv.ParseInt(fields[idx], 10, 64); idx++
	tradingClass := fields[idx]; idx++
	multiplier := fields[idx]; idx++

	if idx >= len(fields) { return }
	expCount, _ := strconv.Atoi(fields[idx]); idx++
	expirations := make([]string, 0, expCount)
	for i := 0; i < expCount && idx < len(fields); i++ {
		expirations = append(expirations, fields[idx]); idx++
	}

	if idx >= len(fields) { return }
	strikeCount, _ := strconv.Atoi(fields[idx]); idx++
	strikes := make([]float64, 0, strikeCount)
	for i := 0; i < strikeCount && idx < len(fields); i++ {
		s, _ := strconv.ParseFloat(fields[idx], 64)
		strikes = append(strikes, s)
		idx++
	}

	d.wrapper.SecurityDefinitionOptionParameter(reqId, exchange, underlyingConId, tradingClass, multiplier, expirations, strikes)
}

func (d *Decoder) handleTickOptionComputation(fields []string) {
	idx := 1

	version := math.MaxInt32
	if d.serverVersion < minServerVerPriceBasedVolatility {
		if idx >= len(fields) { return }
		version, _ = strconv.Atoi(fields[idx]); idx++
	}

	if idx+2 > len(fields) { return }
	reqId, _ := strconv.ParseInt(fields[idx], 10, 64); idx++
	tickType, _ := strconv.Atoi(fields[idx]); idx++

	tickAttrib := 0
	if d.serverVersion >= minServerVerPriceBasedVolatility {
		if idx >= len(fields) { return }
		tickAttrib, _ = strconv.Atoi(fields[idx]); idx++
	}

	if idx+2 > len(fields) { return }
	impliedVol, _ := strconv.ParseFloat(fields[idx], 64); idx++
	delta, _ := strconv.ParseFloat(fields[idx], 64); idx++

	if impliedVol == -1 { impliedVol = math.MaxFloat64 }
	if delta == -2 { delta = math.MaxFloat64 }

	optPrice := math.MaxFloat64
	pvDividend := math.MaxFloat64
	gamma := math.MaxFloat64
	vega := math.MaxFloat64
	theta := math.MaxFloat64
	undPrice := math.MaxFloat64

	if version >= 6 || tickType == 13 || tickType == 83 {
		if idx+2 > len(fields) { goto emit }
		optPrice, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if optPrice == -1 { optPrice = math.MaxFloat64 }
		pvDividend, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if pvDividend == -1 { pvDividend = math.MaxFloat64 }
	}

	if version >= 6 || tickType == 13 || tickType == 83 {
		if idx+3 > len(fields) { goto emit }
		gamma, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if gamma == -2 { gamma = math.MaxFloat64 }
		vega, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if vega == -2 { vega = math.MaxFloat64 }
		theta, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if theta == -2 { theta = math.MaxFloat64 }
	}

	if version >= 6 || tickType == 13 || tickType == 83 {
		if idx >= len(fields) { goto emit }
		undPrice, _ = strconv.ParseFloat(fields[idx], 64); idx++
		if undPrice == -1 { undPrice = math.MaxFloat64 }
	}

emit:
	d.wrapper.TickOptionComputation(reqId, tickType, tickAttrib, impliedVol, delta, optPrice, pvDividend, gamma, vega, theta, undPrice)
}
