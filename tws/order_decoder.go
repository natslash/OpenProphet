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

	cursor.skip(1) // openClose
	cursor.skip(1) // origin
	cursor.skip(1) // orderRef
	cursor.skip(1) // clientId
	cursor.skip(1) // permId
	cursor.skip(1) // outsideRth
	cursor.skip(1) // hidden
	cursor.skip(1) // discretionaryAmt
	cursor.skip(1) // goodAfterTime
	cursor.skip(1) // sharesAllocation
	cursor.skip(3) // faGroup, faMethod, faPercentage
	cursor.skip(1) // modelCode
	cursor.skip(1) // goodTillDate
	cursor.skip(1) // rule80A
	cursor.skip(1) // percentOffset
	cursor.skip(1) // settlingFirm
	cursor.skip(4) // shortSaleSlot, designatedLocation, exemptCode, _ 
	cursor.skip(1) // auctionStrategy
	cursor.skip(2) // startingPrice, stockRefPrice (box order)
	cursor.skip(4) // pegToStkOrVol (delta, referencePriceType...)
	cursor.skip(1) // displaySize
	cursor.skip(1) // blockOrder
	cursor.skip(1) // sweepToFill
	cursor.skip(1) // allOrNone
	cursor.skip(1) // minQty
	cursor.skip(1) // ocaType
	cursor.skip(1) // eTradeOnly
	cursor.skip(1) // firmQuoteOnly
	cursor.skip(1) // nbboPriceCap
	cursor.skip(1) // parentId
	cursor.skip(1) // triggerMethod

	// VolOrderParams
	cursor.skip(1) // volatility
	cursor.skip(1) // volatilityType
	cursor.skip(1) // deltaNeutralOrderType
	cursor.skip(1) // deltaNeutralAuxPrice
	
	deltaNeutralConId := cursor.nextInt()
	if deltaNeutralConId > 0 {
		cursor.skip(1) // deltaNeutralDelta
		cursor.skip(1) // deltaNeutralPrice
		cursor.skip(1) // deltaNeutralShortSale
		cursor.skip(1) // deltaNeutralShortSaleSlot
		cursor.skip(1) // deltaNeutralDesignatedLocation
	}
	cursor.skip(1) // continuousUpdate
	cursor.skip(1) // referencePriceType

	cursor.skip(2) // trailStopPrice, trailingPercent
	cursor.skip(2) // basisPoints, basisPointsType
	
	// ComboLegs
	comboLegsStr := cursor.next()
	comboLegsCount, _ := strconv.Atoi(comboLegsStr)
	if comboLegsCount > 0 {
		for i := 0; i < comboLegsCount; i++ {
			cursor.skip(1) // conId
			cursor.skip(1) // ratio
			cursor.skip(1) // action
			cursor.skip(1) // exchange
			cursor.skip(1) // openClose
			cursor.skip(1) // shortSaleSlot
			cursor.skip(1) // designatedLocation
			cursor.skip(1) // exemptCode
		}
	}

	orderComboLegsCount := cursor.nextInt()
	if orderComboLegsCount > 0 {
		for i := 0; i < orderComboLegsCount; i++ {
			cursor.skip(1) // price
		}
	}

	// SmartComboRoutingParams
	smartComboRoutingParamsCount := cursor.nextInt()
	if smartComboRoutingParamsCount > 0 {
		for i := 0; i < smartComboRoutingParamsCount; i++ {
			cursor.skip(2) // tag, value
		}
	}

	// ScaleOrderParams
	cursor.skip(1) // scaleInitLevelSize
	cursor.skip(1) // scaleSubsLevelSize
	cursor.skip(1) // scalePriceIncrement

	// HedgeParams
	hedgeType := cursor.next()
	if hedgeType != "" {
		cursor.skip(1) // hedgeParam
	}

	cursor.skip(1) // optOutSmartRouting
	cursor.skip(2) // clearingAccount, clearingIntent
	cursor.skip(1) // notHeld

	deltaNeutralContractPresent := cursor.next() == "1"
	if deltaNeutralContractPresent {
		cursor.skip(3) // conId, delta, price
	}

	// AlgoParams
	algoStrategy := cursor.next()
	if algoStrategy != "" {
		algoParamsCount := cursor.nextInt()
		if algoParamsCount > 0 {
			for i := 0; i < algoParamsCount; i++ {
				cursor.skip(2) // tag, value
			}
		}
	}

	cursor.skip(1) // solicited

	// WhatIfInfoAndCommission
	cursor.skip(1) // whatIf

	// FINALLY! OrderState!
	os.Status = cursor.next()
	
	return orderId, c, o, os, true
}
