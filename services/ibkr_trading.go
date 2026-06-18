package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"strconv"
	"time"
)

type IBKRTradingService struct {
	client *tws.Client
}

// Ensure IBKRTradingService implements interfaces.TradingService
var _ interfaces.TradingService = (*IBKRTradingService)(nil)

func NewIBKRTradingService(client *tws.Client) *IBKRTradingService {
	return &IBKRTradingService{client: client}
}

func (s *IBKRTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	return nil, fmt.Errorf("PlaceOrder not implemented in Phase 3")
}

func (s *IBKRTradingService) CancelOrder(ctx context.Context, orderID string) error {
	return fmt.Errorf("CancelOrder not implemented in Phase 3")
}

func (s *IBKRTradingService) GetOrder(ctx context.Context, orderID string) (*interfaces.Order, error) {
	orders, err := s.ListOrders(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		if o.ID == orderID {
			return o, nil
		}
	}
	return nil, fmt.Errorf("order %s not found", orderID)
}

func (s *IBKRTradingService) ListOrders(ctx context.Context, status string) ([]*interfaces.Order, error) {
	ch := s.client.Register(0) // Dispatcher uses 0 for global events like OpenOrders
	defer s.client.Complete(0)

	if err := s.client.Encoder().ReqOpenOrders(); err != nil {
		return nil, fmt.Errorf("ReqOpenOrders error: %w", err)
	}

	var orders []*interfaces.Order
	
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by dispatcher.Complete(0) upon receiving OpenOrderEndMsg
				if status != "" {
					var filtered []*interfaces.Order
					for _, o := range orders {
						// Simple active/inactive filtering
						isActive := false
						if o.Status == "Submitted" || o.Status == "PreSubmitted" || o.Status == "PendingSubmit" || o.Status == "PendingCancel" {
							isActive = true
						}
						
						if status == "open" && isActive {
							filtered = append(filtered, o)
						} else if status == "closed" && !isActive {
							filtered = append(filtered, o)
						} else if status == o.Status {
							filtered = append(filtered, o)
						}
					}
					return filtered, nil
				}
				return orders, nil
			}
			
			switch t := msg.(type) {
			case tws.OpenOrderMsg:
				o := &interfaces.Order{
					ID:     strconv.FormatInt(t.OrderId, 10),
					Symbol: t.Contract.Symbol,
					Qty:    t.Order.TotalQuantity.InexactFloat64(),
					Side:   t.Order.Action,
					Type:   t.Order.OrderType,
					Status: t.OrderState.Status,
				}
				orders = append(orders, o)
			case tws.OpenOrderEndMsg:
				// Ignored, handled by channel close
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRTradingService) GetPositions(ctx context.Context) ([]*interfaces.Position, error) {
	ch := s.client.Register(0)
	defer s.client.Complete(0)

	if err := s.client.Encoder().ReqPositions(); err != nil {
		return nil, fmt.Errorf("ReqPositions error: %w", err)
	}

	var positions []*interfaces.Position

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by dispatcher.Complete(0) upon receiving PositionEndMsg
				return positions, nil
			}
			
			switch t := msg.(type) {
			case tws.PositionMsg:
				// Filter out zero-positions
				qty := t.Position.InexactFloat64()
				if qty == 0 {
					continue
				}
				p := &interfaces.Position{
					Symbol:        t.Contract.Symbol,
					Qty:           qty,
					AvgEntryPrice: t.AvgCost,
				}
				positions = append(positions, p)
			case tws.PositionEndMsg:
				// Ignored, handled by channel close
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRTradingService) GetAccount(ctx context.Context) (*interfaces.Account, error) {
	reqId := s.client.NextOrderId()
	ch := s.client.Register(reqId)
	defer s.client.Complete(reqId)

	if err := s.client.Encoder().ReqAccountSummary(reqId, "All", "$LEDGER"); err != nil {
		return nil, fmt.Errorf("ReqAccountSummary error: %w", err)
	}
	defer s.client.Encoder().CancelAccountSummary(reqId)

	acc := &interfaces.Account{
		ID: "IBKR_PAPER", // Fallback ID
	}
	
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by dispatcher.Complete(reqId) upon receiving AccountSummaryEndMsg
				return acc, nil
			}
			
			switch t := msg.(type) {
			case tws.AccountSummaryMsg:
				acc.ID = t.Account
				if t.Tag == "NetLiquidationByCurrency" {
					if val, err := strconv.ParseFloat(t.Value, 64); err == nil {
						acc.PortfolioValue = val
					}
				} else if t.Tag == "TotalCashBalance" || t.Tag == "CashBalance" {
					if val, err := strconv.ParseFloat(t.Value, 64); err == nil {
						acc.Cash = val
					}
				} else if t.Tag == "BuyingPower" {
					if val, err := strconv.ParseFloat(t.Value, 64); err == nil {
						acc.BuyingPower = val
					}
				} else if t.Tag == "DayTradesRemaining" {
					if val, err := strconv.Atoi(t.Value); err == nil {
						acc.DayTradeCount = val
					}
				}
			case tws.AccountSummaryEndMsg:
				// Ignored, handled by channel close
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRTradingService) PlaceOptionsOrder(ctx context.Context, order *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	return nil, fmt.Errorf("PlaceOptionsOrder not implemented in Phase 3")
}

func (s *IBKRTradingService) GetOptionsChain(ctx context.Context, underlying string, expiration time.Time) ([]*interfaces.OptionContract, error) {
	return nil, fmt.Errorf("GetOptionsChain not implemented in Phase 3")
}

func (s *IBKRTradingService) GetOptionsQuote(ctx context.Context, symbol string) (*interfaces.OptionsQuote, error) {
	return nil, fmt.Errorf("GetOptionsQuote not implemented in Phase 3")
}

func (s *IBKRTradingService) GetOptionsPosition(ctx context.Context, symbol string) (*interfaces.OptionsPosition, error) {
	return nil, fmt.Errorf("GetOptionsPosition not implemented in Phase 3")
}

func (s *IBKRTradingService) ListOptionsPositions(ctx context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, fmt.Errorf("ListOptionsPositions not implemented in Phase 3")
}
