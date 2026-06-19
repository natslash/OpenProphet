package services

import (
	"context"
	"fmt"
	"log"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"strconv"
	"strings"
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

// normalizeOrderType maps the interface's order-type strings (Alpaca-style,
// lower-case) to TWS order-type codes. An empty or unrecognized type is
// rejected rather than silently defaulting to a market order (guardrail:
// never send a market order implicitly).
func normalizeOrderType(t string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "market", "mkt":
		return "MKT", nil
	case "limit", "lmt":
		return "LMT", nil
	case "stop", "stp":
		return "STP", nil
	case "stop_limit", "stop limit", "stp lmt":
		return "STP LMT", nil
	case "":
		return "", fmt.Errorf("order type is required (market/limit/stop); refusing to default to a market order")
	default:
		return "", fmt.Errorf("unsupported order type %q", t)
	}
}

func (s *IBKRTradingService) PlaceOrder(ctx context.Context, order *interfaces.Order) (*interfaces.OrderResult, error) {
	contract, err := tws.ParseSymbol(order.Symbol)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder: %w", err)
	}

	parentID := s.client.NextOrderId()

	side := "BUY"
	reverseSide := "SELL"
	if strings.EqualFold(order.Side, "sell") {
		side = "SELL"
		reverseSide = "BUY"
	}

	orderType, err := normalizeOrderType(order.Type)
	if err != nil {
		return nil, fmt.Errorf("PlaceOrder: %w", err)
	}

	parentOrder := tws.Order{
		Action:        side,
		TotalQuantity: decimal.NewFromFloat(order.Qty),
		OrderType:     orderType,
		Tif:           order.TimeInForce,
		LmtPrice:      tws.UnsetFloat,
		AuxPrice:      tws.UnsetFloat,
	}

	if order.LimitPrice != nil {
		parentOrder.LmtPrice = *order.LimitPrice
	}
	if order.StopPrice != nil {
		parentOrder.AuxPrice = *order.StopPrice
	}
	if parentOrder.Tif == "" {
		parentOrder.Tif = "DAY"
	}

	var tpID, slID int64
	var tpOrder, slOrder tws.Order

	if order.TakeProfitPrice != nil {
		tpID = s.client.NextOrderId()
		tpOrder = tws.Order{
			Action:        reverseSide,
			TotalQuantity: parentOrder.TotalQuantity,
			OrderType:     "LMT",
			Tif:           parentOrder.Tif,
			LmtPrice:      *order.TakeProfitPrice,
			AuxPrice:      tws.UnsetFloat,
			ParentId:      parentID,
		}
	}

	if order.StopLossPrice != nil {
		slID = s.client.NextOrderId()
		slOrder = tws.Order{
			Action:        reverseSide,
			TotalQuantity: parentOrder.TotalQuantity,
			OrderType:     "STP",
			Tif:           parentOrder.Tif,
			LmtPrice:      tws.UnsetFloat,
			AuxPrice:      *order.StopLossPrice,
			ParentId:      parentID,
		}
	}

	// Transmit logic
	if tpID == 0 && slID == 0 {
		parentOrder.Transmit = true
	} else if slID > 0 {
		parentOrder.Transmit = false
		if tpID > 0 {
			tpOrder.Transmit = false
		}
		slOrder.Transmit = true
	} else if tpID > 0 {
		parentOrder.Transmit = false
		tpOrder.Transmit = true
	}

	// Guardrail: log the full intended order BEFORE it hits the socket.
	lmtStr := "unset"
	if order.LimitPrice != nil {
		lmtStr = fmt.Sprintf("%.4f", *order.LimitPrice)
	}
	log.Printf("[IBKR] ORDER INTENT id=%d %s %s qty=%v type=%s lmt=%s tif=%s tp=%v sl=%v (paper)",
		parentID, side, order.Symbol, order.Qty, orderType, lmtStr, parentOrder.Tif, tpID > 0, slID > 0)

	// Register channels for all legs
	type resultMsg struct {
		id  int64
		msg any
	}
	// Buffer must be large enough to hold all initial statuses/errors without blocking goroutines
	aggCh := make(chan resultMsg, 30)

	ids := []int64{parentID}
	if tpID > 0 {
		ids = append(ids, tpID)
	}
	if slID > 0 {
		ids = append(ids, slID)
	}

	for _, id := range ids {
		id := id // capture
		ch := s.client.Register(id)
		defer s.client.Complete(id)
		go func() {
			for msg := range ch {
				aggCh <- resultMsg{id: id, msg: msg}
			}
		}()
	}

	// Emit orders. If any fail to encode/send, we must attempt to cancel the parent to prevent orphans.
	if err := s.client.Encoder().PlaceOrder(s.client.ServerVersion(), parentID, contract, parentOrder); err != nil {
		return nil, fmt.Errorf("PlaceOrder parent encode/send failed: %w", err)
	}
	if tpID > 0 {
		if err := s.client.Encoder().PlaceOrder(s.client.ServerVersion(), tpID, contract, tpOrder); err != nil {
			s.cancelForCleanup(parentID, "TP send failed")
			return nil, fmt.Errorf("PlaceOrder TP encode/send failed: %w", err)
		}
	}
	if slID > 0 {
		if err := s.client.Encoder().PlaceOrder(s.client.ServerVersion(), slID, contract, slOrder); err != nil {
			s.cancelForCleanup(parentID, "SL send failed")
			return nil, fmt.Errorf("PlaceOrder SL encode/send failed: %w", err)
		}
	}

	// Wait for the first authoritative response: orderStatus (accepted) or an error (rejected).
	// We wait on aggCh to catch errors on ANY leg, but success requires parent confirmation.
	confirmCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		select {
		case rm := <-aggCh:
			switch t := rm.msg.(type) {
			case tws.OrderStatusMsg:
				if rm.id == parentID {
					var tpStr, slStr string
					if tpID > 0 {
						tpStr = fmt.Sprintf("%d", tpID)
					}
					if slID > 0 {
						slStr = fmt.Sprintf("%d", slID)
					}

					return &interfaces.OrderResult{
						OrderID:           fmt.Sprintf("%d", parentID),
						TakeProfitOrderID: tpStr,
						StopLossOrderID:   slStr,
						Status:            "submitted",
						Message:           fmt.Sprintf("Parent order %d submitted; awaiting fills", parentID),
					}, nil
				}
			case error:
				// On any rejection, tear down the group to avoid orphan legs.
				s.cancelForCleanup(parentID, fmt.Sprintf("leg %d rejected", rm.id))
				return nil, fmt.Errorf("order %d rejected by TWS: %w", rm.id, t)
			}
		case <-confirmCtx.Done():
			var tpStr, slStr string
			if tpID > 0 {
				tpStr = fmt.Sprintf("%d", tpID)
			}
			if slID > 0 {
				slStr = fmt.Sprintf("%d", slID)
			}

			// Sent but unacknowledged.
			return &interfaces.OrderResult{
				OrderID:           strconv.FormatInt(parentID, 10),
				TakeProfitOrderID: tpStr,
				StopLossOrderID:   slStr,
				Status:            "PendingConfirm",
				Message:           "Order sent; no acknowledgement yet — reconcile via ListOrders",
			}, nil
		}
	}
}

// cancelForCleanup best-effort cancels the parent order (which cascades to any
// attached children) to avoid orphan legs when a bracket can't be fully
// submitted or is rejected. Logged for the order-audit trail; the cancel error
// is ignored because this runs on an already-failing path.
func (s *IBKRTradingService) cancelForCleanup(parentID int64, reason string) {
	log.Printf("[IBKR] CLEANUP CANCEL id=%d: %s (cancel parent; cascades to children)", parentID, reason)
	_ = s.client.Encoder().CancelOrder(s.client.ServerVersion(), parentID)
}

func (s *IBKRTradingService) CancelOrder(ctx context.Context, orderID string) error {
	reqId, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid orderID %q: %w", orderID, err)
	}
	log.Printf("[IBKR] CANCEL INTENT id=%d (paper)", reqId)
	if err := s.client.Encoder().CancelOrder(s.client.ServerVersion(), reqId); err != nil {
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
					Symbol: tws.FormatSymbol(t.Contract),
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
					Symbol:        tws.FormatSymbol(t.Contract),
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
