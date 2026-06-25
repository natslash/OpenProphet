package tws

// Outbound message IDs
const (
	outReqMktData           = 1
	outCancelMktData        = 2
	outPlaceOrder           = 3
	outCancelOrder          = 4
	outReqOpenOrders        = 5
	outReqAccountData       = 6
	outReqAccountSummary    = 62
	outCancelAccountSummary = 63
	outReqPositions         = 61
	outCancelPositions      = 64
	outReqContractDetails   = 9
	outReqHistoricalData    = 20
	outReqCurrentTime       = 49
	outReqMarketDataType    = 59
	outStartAPI             = 71
	outReqSecDefOptParams   = 78
)

// Inbound message IDs
const (
	inTickPrice       = 1
	inTickSize        = 2
	inOrderStatus     = 3
	inErrMsg          = 4
	inOpenOrder       = 5
	inAcctValue         = 6
	inPortfolioValue    = 7
	inAcctUpdateTime    = 8
	inNextValidID       = 9
	inContractData    = 10
	inExecutionData   = 11
	inManagedAccts    = 15
	inHistoricalData  = 17
	inTickOptionComp  = 21
	inCurrentTime     = 49
	inContractDataEnd       = 52
	inOpenOrderEnd          = 53
	inSecDefOptParam        = 75
	inSecDefOptParamEnd     = 76
	inPosition          = 61
	inPositionEnd       = 62
	inAcctDownloadEnd   = 54
	inAccountSummary    = 63
	inAccountSummaryEnd = 64
	inHistoricalDataUpdate = 90
	inHistoricalDataEnd    = 108
	inMarketDataType       = 58
)

// Server-version gates that affect placeOrder field emission.
// Values are the canonical MIN_SERVER_VER_* numbers from the TWS API
// (IBJts/source/JavaClient/.../EClient.java). A field block is emitted
// when the negotiated serverVersion >= the gate.
const (
	minServerVerScaleOrders          = 35
	minServerVerWhatIfOrders         = 36
	minServerVerPtaOrders            = 39
	minServerVerDeltaNeutral         = 40
	minServerVerScaleOrders2         = 40
	minServerVerAlgoOrders           = 41
	minServerVerNotHeld              = 44
	minServerVerSecIdType            = 45
	minServerVerPlaceOrderConId      = 46
	minServerVerSshortxOld           = 51
	minServerVerHedgeOrders          = 54
	minServerVerOptOutSmartRouting   = 56
	minServerVerDeltaNeutralConId    = 58
	minServerVerScaleOrders3         = 60
	minServerVerOrderComboLegsPrice  = 61
	minServerVerTrailingPercent      = 62
	minServerVerTradingClass         = 68
	minServerVerScaleTable           = 69
	minServerVerLinking              = 70
	minServerVerAlgoId               = 71
	minServerVerOrderSolicited       = 73
	minServerVerRandomizeSizeAndPrice = 76
	minServerVerFractionalPositions  = 101
	minServerVerPeggedToBenchmark    = 102
	minServerVerModelsSupport        = 103
	minServerVerExtOperator          = 105
	minServerVerSoftDollarTier       = 106
	minServerVerCashQty              = 111
	minServerVerDecisionMaker        = 138
	minServerVerMifidExecution       = 139
	minServerVerAutoPriceForHedge    = 141
	minServerVerOrderContainer       = 145
	minServerVerDPegOrders           = 148
	minServerVerPriceMgmtAlgo        = 151
	minServerVerPriceBasedVolatility = 156
	minServerVerDuration             = 158
	minServerVerPostToAts            = 160
	minServerVerAutoCancelParent     = 162
	minServerVerAdvancedOrderReject  = 166
	minServerVerManualOrderTime      = 169
	minServerVerPegbestPegmidOffsets = 170
	minServerVerFaProfileDesupport   = 177
	minServerVerCustomerAccount      = 183
	minServerVerProfessionalCustomer = 184
	minServerVerRfqFields            = 187
	minServerVerUndoRfqFields        = 190
	minServerVerIncludeOvernight     = 189
	minServerVerCmeTaggingFields     = 192
	minServerVerImbalanceOnly        = 199
)

// Sentinel string for an unset double, matching the TWS reference clients'
// str(DBL_MAX) emission for non-handle-empty double fields.
const unsetDoubleStr = "1.7976931348623157e+308"

// unsetIntStr is the value emitted for an unset integer field that is sent
// with the plain (non-handle-empty) encoder, matching str(Integer.MAX_VALUE).
const unsetIntStr = "2147483647"

// Tick types
const (
	TickBidSize   = 0
	TickBidPrice  = 1
	TickAskPrice  = 2
	TickAskSize   = 3
	TickLastPrice = 4
	TickLastSize  = 5
	TickHigh      = 6
	TickLow       = 7
	TickVolume    = 8
	TickClose     = 9

	TickDelayedBid      = 66
	TickDelayedAsk      = 67
	TickDelayedLast     = 68
	TickDelayedBidSize  = 69
	TickDelayedAskSize  = 70
	TickDelayedLastSize = 71
	TickDelayedHigh     = 72
	TickDelayedLow      = 73
	TickDelayedClose    = 75
	TickDelayedOpen     = 76
)
