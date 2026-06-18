package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type IBKRTradingService struct {
	tws.DefaultWrapper
	client      *tws.Client
	globalReqMu sync.Mutex
}

// Ensure IBKRTradingService implements interfaces.TradingService
var _ interfaces.TradingService = (*IBKRTradingService)(nil)

func NewIBKRTradingService(client *tws.Client) *IBKRTradingService {
	s := &IBKRTradingService{client: client}
	client.AddWrapper(s)
	return s
}

func (s *IBKRTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	reqId := s.client.NextOrderId()

	contract := tws.Contract{
		Symbol:   order.Symbol,
		SecType:  "STK", // Hardcoded for now
		Exchange: "SMART",
		Currency: "USD",
	}

	side := "BUY"
	if order.Side == "sell" {
		side = "SELL"
	}

	orderType := "MKT"
	if order.Type != "" {
		orderType = order.Type
	}

	twsOrder := tws.Order{
		Action:        side,
		OrderType:     orderType,
		Tif:           order.TimeInForce,
	}
	
	// Convert float64 Qty to decimal
	twsOrder.TotalQuantity = decimal.NewFromFloat(order.Qty)

	if order.LimitPrice != nil {
		twsOrder.LmtPrice = *order.LimitPrice
	}
	if order.StopPrice != nil {
		twsOrder.AuxPrice = *order.StopPrice
	}

	if twsOrder.Tif == "" {
		twsOrder.Tif = "DAY"
	}

	if err := s.client.Encoder().PlaceOrder(reqId, contract, twsOrder); err != nil {
		return nil, fmt.Errorf("PlaceOrder failed: %w", err)
	}

	return &interfaces.OrderResult{
		OrderID: strconv.FormatInt(reqId, 10),
		Status:  "Submitted",
		Message: "Order placed via TWS API",
	}, nil
}

func (s *IBKRTradingService) CancelOrder(ctx context.Context, orderID string) error {
	reqId, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid orderID %q: %w", orderID, err)
	}
	if err := s.client.Encoder().CancelOrder(reqId); err != nil {
		return fmt.Errorf("CancelOrder failed: %w", err)
	}
	return nil
}

func (s *IBKRTradingService) OpenOrder(orderId int64, contract tws.Contract, order tws.Order, orderState tws.OrderState) {
	fmt.Printf("[IBKRTradingService] OpenOrder: %d %s %s %s -> %s\n", orderId, order.Action, order.TotalQuantity.String(), contract.Symbol, orderState.Status)
}

func (s *IBKRTradingService) OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64) {
	fmt.Printf("[IBKRTradingService] OrderStatus: %d -> %s (Filled: %s, Remaining: %s)\n", orderId, status, filled.String(), remaining.String())
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
	s.globalReqMu.Lock()
	defer s.globalReqMu.Unlock()

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
	s.globalReqMu.Lock()
	defer s.globalReqMu.Unlock()

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

	if err := s.client.Encoder().ReqAccountSummary(reqId, "All", "NetLiquidation,TotalCashValue,BuyingPower,DayTradesRemaining"); err != nil {
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
				if t.Tag == "NetLiquidation" {
					if val, err := strconv.ParseFloat(t.Value, 64); err == nil {
						acc.PortfolioValue = val
					}
				} else if t.Tag == "TotalCashValue" {
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
