package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type IBKRDataService struct {
	tws.DefaultWrapper
	client  *tws.Client
	mu      sync.RWMutex
	streams map[int64]chan any
}

// Ensure IBKRDataService implements interfaces.DataService
var _ interfaces.DataService = (*IBKRDataService)(nil)

func NewIBKRDataService(client *tws.Client) *IBKRDataService {
	s := &IBKRDataService{
		client:  client,
		streams: make(map[int64]chan any),
	}
	client.AddWrapper(s)
	return s
}

// symbolToContract resolves an interface Symbol via the shared symbology
// convention (US stock by default, "OESX:<expiry>:<C|P>:<strike>" for OESX).
func symbolToContract(symbol string) (tws.Contract, error) {
	return tws.ParseSymbol(symbol)
}

func (s *IBKRDataService) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr tws.TickAttrib) {
	s.mu.RLock()
	ch, ok := s.streams[reqId]
	s.mu.RUnlock()
	if ok {
		select {
		case ch <- tws.TickPriceMsg{TickType: tickType, Price: price, Size: size, Attr: attr}:
		default:
		}
	}
}

func (s *IBKRDataService) TickSize(reqId int64, tickType int, size decimal.Decimal) {
	s.mu.RLock()
	ch, ok := s.streams[reqId]
	s.mu.RUnlock()
	if ok {
		select {
		case ch <- tws.TickSizeMsg{TickType: tickType, Size: size}:
		default:
		}
	}
}

func (s *IBKRDataService) Error(reqId int, code int, msg string) {
	s.mu.RLock()
	ch, ok := s.streams[int64(reqId)]
	s.mu.RUnlock()
	if ok {
		select {
		case ch <- fmt.Errorf("tws error: %d %s", code, msg):
		default:
		}
	}
}

func (s *IBKRDataService) subscribe(reqId int64) chan any {
	ch := make(chan any, 1024)
	s.mu.Lock()
	s.streams[reqId] = ch
	s.mu.Unlock()
	return ch
}

func (s *IBKRDataService) unsubscribe(reqId int64) {
	s.mu.Lock()
	if ch, ok := s.streams[reqId]; ok {
		close(ch)
		delete(s.streams, reqId)
	}
	s.mu.Unlock()
}

func (s *IBKRDataService) GetHistoricalBars(ctx context.Context, symbol string, start, end time.Time, timeframe string) ([]*interfaces.Bar, error) {
	return nil, fmt.Errorf("GetHistoricalBars not implemented in Phase 3")
}

func (s *IBKRDataService) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	return nil, fmt.Errorf("GetLatestBar not implemented in Phase 3")
}

func (s *IBKRDataService) StreamBars(ctx context.Context, symbols []string) (<-chan *interfaces.Bar, error) {
	return nil, fmt.Errorf("StreamBars not implemented in Phase 3")
}

func (s *IBKRDataService) GetLatestQuote(ctx context.Context, symbol string) (*interfaces.Quote, error) {
	contract, err := symbolToContract(symbol)
	if err != nil {
		return nil, fmt.Errorf("GetLatestQuote: %w", err)
	}

	reqId := s.client.NextOrderId()
	ch := s.subscribe(reqId)
	defer s.unsubscribe(reqId)

	// Subscribe to live market data (snapshot = false since we want to wait for streams if needed,
	// though a snapshot might be cleaner, let's use continuous to get real ticks)
	if err := s.client.Encoder().ReqMktData(reqId, contract, "", false, false); err != nil {
		return nil, fmt.Errorf("ReqMktData error: %w", err)
	}
	defer s.client.Encoder().CancelMktData(reqId)

	quote := &interfaces.Quote{
		Symbol:    symbol,
		Timestamp: time.Now(),
	}

	var hasBid, hasAsk bool

	for {
		// A quote is complete once we have both bid and ask prices
		if hasBid && hasAsk {
			return quote, nil
		}

		select {
		case msg := <-ch:
			switch t := msg.(type) {
			case tws.TickPriceMsg:
				if t.TickType == tws.TickBidPrice {
					quote.BidPrice = t.Price
					if !t.Size.IsZero() {
						quote.BidSize = t.Size.IntPart()
					}
					hasBid = true
				} else if t.TickType == tws.TickAskPrice {
					quote.AskPrice = t.Price
					if !t.Size.IsZero() {
						quote.AskSize = t.Size.IntPart()
					}
					hasAsk = true
				}
			case tws.TickSizeMsg:
				if t.TickType == tws.TickBidSize {
					quote.BidSize = t.Size.IntPart()
				} else if t.TickType == tws.TickAskSize {
					quote.AskSize = t.Size.IntPart()
				}
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRDataService) GetLatestTrade(ctx context.Context, symbol string) (*interfaces.Trade, error) {
	contract, err := symbolToContract(symbol)
	if err != nil {
		return nil, fmt.Errorf("GetLatestTrade: %w", err)
	}

	reqId := s.client.NextOrderId()
	ch := s.subscribe(reqId)
	defer s.unsubscribe(reqId)

	if err := s.client.Encoder().ReqMktData(reqId, contract, "", false, false); err != nil {
		return nil, fmt.Errorf("ReqMktData error: %w", err)
	}
	defer s.client.Encoder().CancelMktData(reqId)

	trade := &interfaces.Trade{
		Symbol:    symbol,
		Timestamp: time.Now(),
	}

	var hasPrice, hasSize bool

	for {
		if hasPrice && hasSize {
			return trade, nil
		}

		select {
		case msg := <-ch:
			switch t := msg.(type) {
			case tws.TickPriceMsg:
				if t.TickType == tws.TickLastPrice {
					trade.Price = t.Price
					if !t.Size.IsZero() {
						trade.Size = t.Size.IntPart()
						hasSize = true
					}
					hasPrice = true
				}
			case tws.TickSizeMsg:
				if t.TickType == tws.TickLastSize {
					trade.Size = t.Size.IntPart()
					hasSize = true
				}
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
