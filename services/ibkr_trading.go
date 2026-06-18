package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"strconv"
	"time"
)

// accountSummaryTags are the tags requested for GetAccount, mapped onto the
// interfaces.Account fields below. PatternDayTrader / DayTradeCount are US
// concepts and are intentionally left zero for IBKR.
const accountSummaryTags = "NetLiquidation,TotalCashValue,BuyingPower"

// IBKRTradingService implements the read-only paths of interfaces.TradingService
// by wrapping the TWS client.
type IBKRTradingService struct {
	client *tws.Client
}

// Ensure IBKRTradingService implements interfaces.TradingService
var _ interfaces.TradingService = (*IBKRTradingService)(nil)

func NewIBKRTradingService(client *tws.Client) *IBKRTradingService {
	return &IBKRTradingService{client: client}
}

// GetAccount returns the account summary by requesting NetLiquidation,
// TotalCashValue and BuyingPower from TWS and mapping them onto
// interfaces.Account. US-specific fields (PatternDayTrader, DayTradeCount) are
// left zero/false.
func (s *IBKRTradingService) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	reqId := s.client.NextOrderId()
	ch := s.client.Register(reqId)
	defer s.client.Complete(reqId)

	if err := s.client.Encoder().ReqAccountSummary(reqId, "All", accountSummaryTags); err != nil {
		return nil, fmt.Errorf("ReqAccountSummary error: %w", err)
	}
	defer func() { _ = s.client.Encoder().CancelAccountSummary(reqId) }()

	account := &interfaces.Account{}

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by AccountSummaryEnd: all tags delivered.
				return account, nil
			}
			switch m := msg.(type) {
			case tws.AccountSummaryMsg:
				if account.ID == "" {
					account.ID = m.Account
				}
				val, err := strconv.ParseFloat(m.Value, 64)
				if err != nil {
					continue
				}
				switch m.Tag {
				case "NetLiquidation":
					account.PortfolioValue = val
				case "TotalCashValue":
					account.Cash = val
				case "BuyingPower":
					account.BuyingPower = val
				}
			case error:
				return nil, fmt.Errorf("tws error: %w", m)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// GetPositions returns the current positions for the account. De-activated /
// zero-quantity records are skipped.
func (s *IBKRTradingService) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	ch, err := s.client.ReqPositions()
	if err != nil {
		return nil, fmt.Errorf("ReqPositions error: %w", err)
	}
	defer func() { _ = s.client.CancelPositions() }()

	var positions []*interfaces.Position

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by PositionEnd: all positions delivered.
				return positions, nil
			}
			p, isPos := msg.(tws.PositionMsg)
			if !isPos {
				continue
			}
			qty := p.Position.InexactFloat64()
			if qty == 0 {
				continue
			}
			side := "long"
			if qty < 0 {
				side = "short"
			}
			positions = append(positions, &interfaces.Position{
				Symbol:        positionSymbol(p.Contract),
				Qty:           qty,
				AvgEntryPrice: p.AvgCost,
				CostBasis:     p.AvgCost * qty,
				Side:          side,
			})
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// positionSymbol derives a stable interface-level symbol string for a TWS
// contract. For options it includes the underlying, expiry, right and strike;
// otherwise it falls back to LocalSymbol or Symbol.
func positionSymbol(c tws.Contract) string {
	if c.SecType == tws.Option {
		return fmt.Sprintf("%s %s %s%s", c.Symbol, c.LastTradeDateOrContractMonth, c.Right,
			strconv.FormatFloat(c.Strike, 'f', -1, 64))
	}
	if c.LocalSymbol != "" {
		return c.LocalSymbol
	}
	return c.Symbol
}

// ListOrders returns the list of open orders (not yet implemented).
func (s *IBKRTradingService) ListOrders(ctx context.Context, status string) ([]*interfaces.Order, error) {
	return nil, fmt.Errorf("ListOrders not yet implemented")
}

// GetOrder returns a specific order (not yet implemented).
func (s *IBKRTradingService) GetOrder(ctx context.Context, orderID string) (*interfaces.Order, error) {
	return nil, fmt.Errorf("GetOrder not implemented")
}

// PlaceOrder is stubbed as not implemented.
func (s *IBKRTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	return nil, fmt.Errorf("PlaceOrder not implemented")
}

// CancelOrder is stubbed as not implemented.
func (s *IBKRTradingService) CancelOrder(ctx context.Context, orderID string) error {
	return fmt.Errorf("CancelOrder not implemented")
}

// PlaceOptionsOrder is stubbed as not implemented.
func (s *IBKRTradingService) PlaceOptionsOrder(ctx context.Context, order *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	return nil, fmt.Errorf("PlaceOptionsOrder not implemented")
}

// GetOptionsChain is stubbed as not implemented.
func (s *IBKRTradingService) GetOptionsChain(ctx context.Context, underlying string, expiration time.Time) ([]*interfaces.OptionContract, error) {
	return nil, fmt.Errorf("GetOptionsChain not implemented")
}

// GetOptionsQuote is stubbed as not implemented.
func (s *IBKRTradingService) GetOptionsQuote(ctx context.Context, symbol string) (*interfaces.OptionsQuote, error) {
	return nil, fmt.Errorf("GetOptionsQuote not implemented")
}

// GetOptionsPosition is stubbed as not implemented.
func (s *IBKRTradingService) GetOptionsPosition(ctx context.Context, symbol string) (*interfaces.OptionsPosition, error) {
	return nil, fmt.Errorf("GetOptionsPosition not implemented")
}

// ListOptionsPositions is stubbed as not implemented.
func (s *IBKRTradingService) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, fmt.Errorf("ListOptionsPositions not implemented")
}
