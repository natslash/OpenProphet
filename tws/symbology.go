package tws

import (
	"fmt"
	"strconv"
	"strings"
)

// Symbology converts between the interface-level string Symbol and the rich
// tws.Contract. This is the single source of truth for the string convention
// (see CLAUDE.md "The broker seam" — OESX has no OCC symbol, so we define a
// stable string form here and decode it back into a full Contract).
//
// Conventions:
//
//	"AAPL"                     → US stock: STK / SMART / USD
//	"EU:DTE"                   → EU stock: STK / SMART / EUR
//	"FUT:OESX:20260619"        → Future: FUT / EUREX / EUR
//	"ESTX50:20260619:C:6325"   → EURO STOXX 50 index option (OESX): underlying
//	                             ESTX50, EUREX, EUR, multiplier 10, tradingClass
//	                             OESX. Form: ESTX50:<YYYYMMDD>:<C|P>:<strike>
//
// A bare symbol (no ":") is treated as a US stock for backward compatibility.
// OCC-style US option symbols are intentionally not parsed — this fork targets
// European/OESX instruments; extend here if that need appears.

const (
	estoxxSymbol      = "ESTX50" // EURO STOXX 50 underlying
	oesxTradingClass  = "OESX"   // EUREX option class on ESTX50
)

// ParseSymbol turns an interface Symbol string into a tws.Contract.
func ParseSymbol(symbol string) (Contract, error) {
	if symbol == "" {
		return Contract{}, fmt.Errorf("empty symbol")
	}
	parts := strings.Split(symbol, ":")

	if len(parts) == 1 {
		return Contract{
			Symbol:   parts[0],
			SecType:  Stock,
			Exchange: "SMART",
			Currency: "USD",
		}, nil
	}

	if parts[0] == "EU" {
		if len(parts) != 2 {
			return Contract{}, fmt.Errorf("invalid EU stock symbol %q: want EU:<TICKER>", symbol)
		}
		return Contract{
			Symbol:   parts[1],
			SecType:  Stock,
			Exchange: "SMART",
			Currency: "EUR",
		}, nil
	}

	if parts[0] == "FUT" {
		if len(parts) != 3 {
			return Contract{}, fmt.Errorf("invalid FUT symbol %q: want FUT:<SYMBOL>:<YYYYMMDD>", symbol)
		}
		return Contract{
			Symbol:                       parts[1],
			SecType:                      Future,
			Exchange:                     "EUREX", // Defaulting to EUREX for European focus
			Currency:                     "EUR",
			LastTradeDateOrContractMonth: parts[2],
		}, nil
	}

	if parts[0] == estoxxSymbol {
		if len(parts) != 4 {
			return Contract{}, fmt.Errorf("invalid ESTX50 option symbol %q: want %s:<YYYYMMDD>:<C|P>:<strike>", symbol, estoxxSymbol)
		}
		expiry := parts[1]
		right := strings.ToUpper(parts[2])
		if right != "C" && right != "P" {
			return Contract{}, fmt.Errorf("invalid ESTX50 right %q in %q: want C or P", parts[2], symbol)
		}
		strike, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return Contract{}, fmt.Errorf("invalid ESTX50 strike %q in %q: %w", parts[3], symbol, err)
		}
		return Contract{
			Symbol:                       estoxxSymbol,
			SecType:                      Option,
			Exchange:                     "EUREX",
			Currency:                     "EUR",
			LastTradeDateOrContractMonth: expiry,
			Strike:                       strike,
			Right:                        right,
			Multiplier:                   "10",
			TradingClass:                 oesxTradingClass,
		}, nil
	}

	return Contract{}, fmt.Errorf("unrecognized symbol %q (want a bare ticker like AAPL, EU:<TICKER>, FUT:<SYMBOL>:<YYYYMMDD>, or %s:<YYYYMMDD>:<C|P>:<strike>)", symbol, estoxxSymbol)
}

// FormatSymbol is the inverse of ParseSymbol for the contract types we map, so
// positions/orders decoded from TWS are presented with the same convention.
func FormatSymbol(c Contract) string {
	if c.SecType == Option && c.TradingClass == oesxTradingClass {
		return fmt.Sprintf("%s:%s:%s:%s", estoxxSymbol,
			c.LastTradeDateOrContractMonth, c.Right,
			strconv.FormatFloat(c.Strike, 'f', -1, 64))
	}
	if c.SecType == Stock && c.Currency == "EUR" {
		return fmt.Sprintf("EU:%s", c.Symbol)
	}
	if c.SecType == Future {
		return fmt.Sprintf("FUT:%s:%s", c.Symbol, c.LastTradeDateOrContractMonth)
	}
	return c.Symbol
}
