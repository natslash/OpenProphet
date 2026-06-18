package tws

import (
	"strconv"
	"github.com/shopspring/decimal"
)

type stringCursor struct {
	fields []string
	pos    int
}

func (c *stringCursor) next() string {
	if c.pos >= len(c.fields) {
		return ""
	}
	val := c.fields[c.pos]
	c.pos++
	return val
}

func (c *stringCursor) nextInt() int {
	val := c.next()
	if val == "" {
		return 0
	}
	parsed, _ := strconv.Atoi(val)
	return parsed
}

func (c *stringCursor) skip(n int) {
	c.pos += n
}

// decodeOpenOrder performs a layout-driven parse of the OpenOrder message.
func decodeOpenOrder(fields []string) (int64, Contract, Order, OrderState, bool) {
	var c Contract
	var o Order
	var os OrderState

	cursor := &stringCursor{fields: fields, pos: 1} // skip msgId

	// In v187 (or MIN_SERVER_VER_ORDER_CONTAINER+), version is not sent.
	// The next field is orderId.
	orderIdRaw := cursor.next()
	orderId, _ := strconv.ParseInt(orderIdRaw, 10, 64)

	// Contract
	c.ConId, _ = strconv.ParseInt(cursor.next(), 10, 64)
	c.Symbol = cursor.next()
	c.SecType = InstrumentType(cursor.next())
	c.LastTradeDateOrContractMonth = cursor.next()
	c.Strike, _ = strconv.ParseFloat(cursor.next(), 64)
	c.Right = cursor.next()
	c.Multiplier = cursor.next()
	c.Exchange = cursor.next()
	c.Currency = cursor.next()
	c.LocalSymbol = cursor.next()
	c.TradingClass = cursor.next()
	
	// order fields
	o.Action = cursor.next()
	o.TotalQuantity, _ = decimal.NewFromString(cursor.next())
	o.OrderType = cursor.next()
	o.LmtPrice, _ = strconv.ParseFloat(cursor.next(), 64)
	o.AuxPrice, _ = strconv.ParseFloat(cursor.next(), 64)
	o.Tif = cursor.next()
	o.OcaGroup = cursor.next()
	o.Account = cursor.next()

	// The remainder of the openOrder message is a long, heavily version-gated
	// run of order parameters (FA, box/peg/vol, combo legs, scale, hedge, algo,
	// conditions, …) followed by the orderState block whose first field is the
	// status. Rather than track every conditional offset (fragile, and easy to
	// desync), we locate the status directly: it is always one of a known set
	// of enum strings that do not collide with the preceding fields' values.
	os.Status = findOrderStatus(fields, cursor.pos)

	return orderId, c, o, os, true
}

// orderStatusValues is the set of status strings TWS emits in orderState /
// orderStatus (see EWrapper / OrderStatus docs).
var orderStatusValues = map[string]bool{
	"PendingSubmit": true,
	"PendingCancel": true,
	"PreSubmitted":  true,
	"Submitted":     true,
	"ApiPending":    true,
	"ApiCancelled":  true,
	"Cancelled":     true,
	"Filled":        true,
	"Inactive":      true,
	"Unknown":       true,
}

// findOrderStatus returns the first field at or after `from` that is a known
// order-status enum, or "" if none is present.
func findOrderStatus(fields []string, from int) string {
	for i := from; i < len(fields); i++ {
		if orderStatusValues[fields[i]] {
			return fields[i]
		}
	}
	return ""
}
