package tws

type InstrumentType string

const (
	Stock  InstrumentType = "STK"
	Option InstrumentType = "OPT"
	Future InstrumentType = "FUT"
	Index  InstrumentType = "IND"
	Bag    InstrumentType = "BAG"
)

type Contract struct {
	ConId        int64
	Symbol       string
	SecType      InstrumentType
	LastTradeDateOrContractMonth string
	Strike       float64
	Right        string
	Multiplier   string
	Exchange     string
	PrimaryExch  string
	Currency     string
	LocalSymbol  string
	TradingClass string
	ComboLegs    []ComboLeg
}

type ComboLeg struct {
	ConId    int64
	Ratio    int
	Action   string // "BUY" or "SELL"
	Exchange string
}

type ContractDetails struct {
	Contract    Contract
	MarketName  string
	MinTick     float64
	OrderTypes  string
	ValidExchanges string
	PriceMagnifier int64
	UnderConId  int64
	LongName    string
	ContractMonth string
	Industry    string
	Category    string
	Subcategory string
	TimeZoneId  string
	TradingHours string
	LiquidHours  string
	EvRule      string
	EvMultiplier float64
	MdSizeMultiplier int64
	AggGroup    int64
	UnderSymbol string
	UnderSecType string
	MarketRuleIds string
	RealExpirationDate string
	LastTradeTime string
	StockType string
}
