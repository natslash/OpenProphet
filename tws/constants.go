package tws

// Outbound message IDs
const (
	outReqMktData           = 1
	outCancelMktData        = 2
	outPlaceOrder           = 3
	outCancelOrder          = 4
	outReqOpenOrders        = 5
	outReqAccountSummary    = 62
	outCancelAccountSummary = 63
	outReqPositions         = 61
	outCancelPositions      = 64
	outReqContractDetails   = 9
	outReqCurrentTime       = 49
	outStartAPI             = 71
)

// Inbound message IDs
const (
	inTickPrice       = 1
	inTickSize        = 2
	inOrderStatus     = 3
	inErrMsg          = 4
	inOpenOrder       = 5
	inAcctValue       = 6
	inNextValidID     = 9
	inContractData    = 10
	inExecutionData   = 11
	inManagedAccts    = 15
	inTickOptionComp  = 21
	inCurrentTime     = 49
	inPosition        = 61
	inAccountSummary  = 63
)
