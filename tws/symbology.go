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
//	"AAPL"                   → US stock: STK / SMART / USD
//	"OESX:20260620:C:5200"   → OESX index option on the EURO STOXX 50
//	                           (underlying ESTX50, EUREX, EUR, multiplier 10,
//	                            tradingClass OESX). Form: OESX:<YYYYMMDD>:<C|P>:<strike>
//
// A bare symbol (no ":") is treated as a US stock for backward compatibility.
// OCC-style US option symbols are intentionally not parsed — this fork targets
// European/OESX instruments; extend here if that need appears.

const oesxPrefix = "OESX"

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

	if parts[0] == oesxPrefix {
		if len(parts) != 4 {
			return Contract{}, fmt.Errorf("invalid OESX symbol %q: want %s:<YYYYMMDD>:<C|P>:<strike>", symbol, oesxPrefix)
		}
		expiry := parts[1]
		right := strings.ToUpper(parts[2])
		if right != "C" && right != "P" {
			return Contract{}, fmt.Errorf("invalid OESX right %q in %q: want C or P", parts[2], symbol)
		}
		strike, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return Contract{}, fmt.Errorf("invalid OESX strike %q in %q: %w", parts[3], symbol, err)
		}
		return Contract{
			Symbol:                       "ESTX50",
			SecType:                      Option,
			Exchange:                     "EUREX",
			Currency:                     "EUR",
			LastTradeDateOrContractMonth: expiry,
			Strike:                       strike,
			Right:                        right,
			Multiplier:                   "10",
			TradingClass:                 oesxPrefix,
		}, nil
	}

	return Contract{}, fmt.Errorf("unrecognized symbol %q", symbol)
}

// FormatSymbol is the inverse of ParseSymbol for the contract types we map, so
// positions/orders decoded from TWS are presented with the same convention.
func FormatSymbol(c Contract) string {
	if c.SecType == Option && c.TradingClass == oesxPrefix {
		return fmt.Sprintf("%s:%s:%s:%s", oesxPrefix,
			c.LastTradeDateOrContractMonth, c.Right,
			strconv.FormatFloat(c.Strike, 'f', -1, 64))
	}
	return c.Symbol
}
