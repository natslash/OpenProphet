package tws

import (
	"math"
	"strconv"
	"strings"
)

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

// ReqMarketDataType sets the market data type for subsequent requests.
// 1 = Live, 2 = Frozen, 3 = Delayed, 4 = Delayed and frozen
func (e *Encoder) ReqMarketDataType(marketDataType int) error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outReqMarketDataType),
		version,
		strconv.Itoa(marketDataType),
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

// ReqHistoricalData requests historical data.
func (e *Encoder) ReqHistoricalData(serverVersion int, reqId int64, contract Contract, endDateTime, durationStr, barSizeSetting, whatToShow string, useRTH int, formatDate int, keepUpToDate bool) error {
	f := []string{strconv.Itoa(outReqHistoricalData)}
	add := func(v ...string) { f = append(f, v...) }

	const minServerVerSyntRealtimeBars = 32

	if serverVersion < minServerVerSyntRealtimeBars {
		add("6") // version
	}

	add(strconv.FormatInt(reqId, 10))

	// send contract fields
	if serverVersion >= minServerVerTradingClass {
		add(strconv.FormatInt(contract.ConId, 10))
	}

	strikeStr := encFloatMax(contract.Strike)
	if contract.Strike == 0.0 {
		strikeStr = "0.0"
	}

	add(contract.Symbol, string(contract.SecType), contract.LastTradeDateOrContractMonth,
		strikeStr, contract.Right, contract.Multiplier, contract.Exchange,
		contract.PrimaryExch, contract.Currency, contract.LocalSymbol)

	if serverVersion >= minServerVerTradingClass {
		add(contract.TradingClass)
	}

	if serverVersion >= 31 {
		add("0") // includeExpired
	}

	if serverVersion >= 20 {
		add(endDateTime, barSizeSetting)
	}

	add(durationStr, strconv.Itoa(useRTH), whatToShow)

	if serverVersion > 16 {
		add(strconv.Itoa(formatDate))
	}

	if contract.SecType == "BAG" {
		add("0") // combo legs not supported
	}

	if serverVersion >= minServerVerSyntRealtimeBars {
		add(encBool(keepUpToDate))
	}

	if serverVersion >= minServerVerLinking {
		add("") // chartOptions
	}

	return e.writer.SendFields(f...)
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

// ReqAccountUpdates subscribes to (or unsubscribes from) account + portfolio
// updates for a given account code. The server streams UpdateAccountValue,
// UpdatePortfolio, and AccountDownloadEnd messages.
func (e *Encoder) ReqAccountUpdates(subscribe bool, acctCode string) error {
	const version = "2"
	sub := "0"
	if subscribe {
		sub = "1"
	}
	return e.writer.SendFields(strconv.Itoa(outReqAccountData), version, sub, acctCode)
}

// ReqPositions requests all positions for all accounts.
func (e *Encoder) ReqPositions() error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outReqPositions),
		version,
	}
	return e.writer.SendFields(fields...)
}

// CancelPositions cancels a previous ReqPositions subscription.
func (e *Encoder) CancelPositions() error {
	const version = "1"
	fields := []string{
		strconv.Itoa(outCancelPositions),
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

// fmtFloat renders a float the way the TWS reference clients do (Java
// String.valueOf / Python str): a whole number keeps a trailing ".0".
func fmtFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEnN") {
		s += ".0"
	}
	return s
}

// UnsetFloat is the sentinel for an unset double order field. Fields holding
// this value are emitted as "" (the handle-empty convention).
const UnsetFloat = math.MaxFloat64

// encFloatMax emits "" for an unset double, else the formatted value.
func encFloatMax(f float64) string {
	if f == UnsetFloat {
		return ""
	}
	return fmtFloat(f)
}

func encBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// PlaceOrder builds the placeOrder message for the negotiated serverVersion,
// gating each field block exactly as the reference TWS clients do
// (EClient.placeOrder). Only the fields needed for the order types we support
// are populated; everything else is sent as its unset/default sentinel.
func (e *Encoder) PlaceOrder(serverVersion int, reqId int64, contract Contract, order Order) error {
	f := []string{strconv.Itoa(outPlaceOrder)}
	add := func(v ...string) { f = append(f, v...) }

	// version field is omitted once the server speaks the order-container protocol
	if serverVersion < minServerVerOrderContainer {
		add("45")
	}
	add(strconv.FormatInt(reqId, 10))

	// --- contract ---
	if serverVersion >= minServerVerPlaceOrderConId {
		add(strconv.FormatInt(contract.ConId, 10))
	}
	add(contract.Symbol, string(contract.SecType), contract.LastTradeDateOrContractMonth,
		fmtFloat(contract.Strike), contract.Right, contract.Multiplier, contract.Exchange,
		contract.PrimaryExch, contract.Currency, contract.LocalSymbol)
	if serverVersion >= minServerVerTradingClass {
		add(contract.TradingClass)
	}
	if serverVersion >= minServerVerSecIdType {
		add("", "") // secIdType, secId
	}

	// --- main order ---
	add(order.Action, order.TotalQuantity.String(), order.OrderType,
		encFloatMax(order.LmtPrice), encFloatMax(order.AuxPrice))

	// --- extended order ---
	add(order.Tif, order.OcaGroup, order.Account, order.OpenClose,
		strconv.Itoa(order.Origin), order.OrderRef, encBool(order.Transmit),
		strconv.FormatInt(order.ParentId, 10),
		encBool(order.BlockOrder), encBool(order.SweepToFill),
		strconv.Itoa(order.DisplaySize), strconv.Itoa(order.TriggerMethod),
		encBool(order.OutsideRth), encBool(order.Hidden))

	// combo legs (BAG only) — not supported yet; nothing emitted for non-BAG.

	// deprecated sharesAllocation, then discretionaryAmt
	add("", "0.0")
	add("", "")                 // goodAfterTime, goodTillDate
	add("", "", "")             // faGroup, faMethod, faPercentage
	if serverVersion < minServerVerFaProfileDesupport {
		add("") // deprecated faProfile
	}
	if serverVersion >= minServerVerModelsSupport {
		add("") // modelCode
	}
	add("0", "")                // shortSaleSlot, designatedLocation
	if serverVersion >= minServerVerSshortxOld {
		add("-1") // exemptCode
	}
	add("0")                    // ocaType
	add("", "", "0", "", "")    // rule80A, settlingFirm, allOrNone, minQty, percentOffset
	add("0", "0", "")           // eTradeOnly, firmQuoteOnly, nbboPriceCap
	add("0")                    // auctionStrategy
	add("", "", "", "", "")     // startingPrice, stockRefPrice, delta, stockRangeLower, stockRangeUpper
	add("0")                    // overridePercentageConstraints
	add("", "", "", "")         // volatility, volatilityType, deltaNeutralOrderType, deltaNeutralAuxPrice
	add("0", "", "")            // continuousUpdate, referencePriceType, trailStopPrice
	if serverVersion >= minServerVerTrailingPercent {
		add("") // trailingPercent
	}
	// scale orders
	if serverVersion >= minServerVerScaleOrders2 {
		add("", "") // scaleInitLevelSize, scaleSubsLevelSize
	}
	add("") // scalePriceIncrement (unset → no scaleOrders3 block)
	if serverVersion >= minServerVerScaleTable {
		add("", "", "") // scaleTable, activeStartTime, activeStopTime
	}
	if serverVersion >= minServerVerHedgeOrders {
		add("") // hedgeType (empty → no hedgeParam)
	}
	if serverVersion >= minServerVerOptOutSmartRouting {
		add("0")
	}
	if serverVersion >= minServerVerPtaOrders {
		add("", "") // clearingAccount, clearingIntent
	}
	if serverVersion >= minServerVerNotHeld {
		add("0")
	}
	if serverVersion >= minServerVerDeltaNeutral {
		add("0") // no deltaNeutralContract
	}
	if serverVersion >= minServerVerAlgoOrders {
		add("") // algoStrategy (empty → no params)
	}
	if serverVersion >= minServerVerAlgoId {
		add("")
	}
	add(encBool(order.WhatIf))
	if serverVersion >= minServerVerLinking {
		add("") // orderMiscOptions
	}
	if serverVersion >= minServerVerOrderSolicited {
		add("0")
	}
	if serverVersion >= minServerVerRandomizeSizeAndPrice {
		add("0", "0")
	}
	if serverVersion >= minServerVerPeggedToBenchmark {
		add("0") // conditions count (none)
		// adjustedOrderType, triggerPrice, lmtPriceOffset, adjustedStopPrice,
		// adjustedStopLimitPrice, adjustedTrailingAmount (unset doubles), adjustableTrailingUnit
		add("", unsetDoubleStr, unsetDoubleStr, unsetDoubleStr, unsetDoubleStr, unsetDoubleStr, "0")
	}
	if serverVersion >= minServerVerExtOperator {
		add("")
	}
	if serverVersion >= minServerVerSoftDollarTier {
		add("", "") // softDollarTier name, value
	}
	if serverVersion >= minServerVerCashQty {
		add(unsetDoubleStr)
	}
	if serverVersion >= minServerVerDecisionMaker {
		add("", "")
	}
	if serverVersion >= minServerVerMifidExecution {
		add("", "")
	}
	if serverVersion >= minServerVerAutoPriceForHedge {
		add("0")
	}
	if serverVersion >= minServerVerOrderContainer {
		add("0") // isOmsContainer
	}
	if serverVersion >= minServerVerDPegOrders {
		add("0")
	}
	if serverVersion >= minServerVerPriceMgmtAlgo {
		add("") // usePriceMgmtAlgo (None → empty)
	}
	if serverVersion >= minServerVerDuration {
		add(unsetIntStr)
	}
	if serverVersion >= minServerVerPostToAts {
		add(unsetIntStr)
	}
	if serverVersion >= minServerVerAutoCancelParent {
		add("0")
	}
	if serverVersion >= minServerVerAdvancedOrderReject {
		add("")
	}
	if serverVersion >= minServerVerManualOrderTime {
		add("")
	}
	if serverVersion >= minServerVerPegbestPegmidOffsets {
		// IBKRATS minTradeQty / PEG BEST / PEG MID offsets — none for our orders.
		if contract.Exchange == "IBKRATS" {
			add("")
		}
	}
	if serverVersion >= minServerVerCustomerAccount {
		add("")
	}
	if serverVersion >= minServerVerProfessionalCustomer {
		add("0")
	}
	if serverVersion >= minServerVerRfqFields && serverVersion < minServerVerUndoRfqFields {
		add("", unsetIntStr)
	}
	if serverVersion >= minServerVerIncludeOvernight {
		add("0")
	}
	if serverVersion >= minServerVerCmeTaggingFields {
		add(unsetIntStr) // manualOrderIndicator
	}
	if serverVersion >= minServerVerImbalanceOnly {
		add("0")
	}

	return e.writer.SendFields(f...)
}

// CancelOrder cancels an active order.
func (e *Encoder) CancelOrder(serverVersion int, orderID int64) error {
	f := []string{strconv.Itoa(outCancelOrder)}
	if serverVersion < minServerVerCmeTaggingFields {
		f = append(f, "1") // legacy version field
	}
	f = append(f, strconv.FormatInt(orderID, 10))
	if serverVersion >= minServerVerManualOrderTime {
		f = append(f, "") // manualOrderCancelTime
	}
	if serverVersion >= minServerVerRfqFields && serverVersion < minServerVerUndoRfqFields {
		f = append(f, "", "", unsetIntStr)
	}
	if serverVersion >= minServerVerCmeTaggingFields {
		f = append(f, "", unsetIntStr) // extOperator, manualOrderIndicator
	}
	return e.writer.SendFields(f...)
}
