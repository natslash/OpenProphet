package tws

import (
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
)

type Decoder struct {
	wrapper Wrapper
}

func NewDecoder(w Wrapper) *Decoder {
	return &Decoder{wrapper: w}
}

func (d *Decoder) Decode(fields []string) error {
	if len(fields) == 0 || fields[0] == "" {
		return nil
	}

	msgID, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("invalid message ID %q: %w", fields[0], err)
	}

	switch msgID {
	case inNextValidID:
		if len(fields) >= 3 {
			if orderID, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.NextValidId(orderID)
			}
		}
	case inTickPrice:
		d.handleTickPrice(fields)
	case inTickSize:
		d.handleTickSize(fields)
	case inManagedAccts:
		if len(fields) >= 3 {
			d.wrapper.ManagedAccounts(fields[2])
		}
	case inErrMsg:
		if len(fields) >= 5 {
			reqID, _ := strconv.Atoi(fields[2])
			code, _ := strconv.Atoi(fields[3])
			d.wrapper.Error(reqID, code, fields[4])
		}
	case inCurrentTime:
		if len(fields) >= 3 {
			if t, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.CurrentTime(t)
			}
		}
	case inContractData:
		d.handleContractData(fields)
	case inContractDataEnd:
		if len(fields) >= 3 {
			if reqId, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.ContractDetailsEnd(reqId)
			}
		}
	case inHistoricalData:
		d.handleHistoricalData(fields)
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

func (d *Decoder) handleHistoricalData(fields []string) {
	if len(fields) < 10 {
		return
	}
	reqId, _ := strconv.ParseInt(fields[1], 10, 64)
	d.wrapper.HistoricalData(reqId, HistoricalData{
		ReqId: reqId,
		Date: fields[2],
		Open: parseF(fields[3]),
		High: parseF(fields[4]),
		Low: parseF(fields[5]),
		Close: parseF(fields[6]),
		Volume: parseF(fields[7]),
		Count: parseInt(fields[8]),
		WAP: parseF(fields[9]),
	})
}

func parseF(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func parseInt(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }

func (d *Decoder) handleTickPrice(fields []string) {
	if len(fields) < 6 { return }
	version, _ := strconv.Atoi(fields[1])
	reqId, _ := strconv.ParseInt(fields[2], 10, 64)
	tickType, _ := strconv.Atoi(fields[3])
	price, _ := strconv.ParseFloat(fields[4], 64)
	size := decimal.Zero
	if version >= 2 { size, _ = decimal.NewFromString(fields[5]) }
	attr := TickAttrib{}
	if version >= 3 && len(fields) >= 7 {
		attrMask, _ := strconv.Atoi(fields[6])
		attr.CanAutoExecute = (attrMask & 1) != 0
		if version >= 4 { attr.PastLimit = (attrMask & 2) != 0 }
		if version >= 6 { attr.PreOpen = (attrMask & 4) != 0 }
	}
	d.wrapper.TickPrice(reqId, tickType, price, size, attr)
}

func (d *Decoder) handleTickSize(fields []string) {
	if len(fields) < 5 { return }
	reqId, _ := strconv.ParseInt(fields[2], 10, 64)
	tickType, _ := strconv.Atoi(fields[3])
	size, _ := decimal.NewFromString(fields[4])
	d.wrapper.TickSize(reqId, tickType, size)
}
