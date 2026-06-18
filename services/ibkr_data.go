package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"time"
)

type IBKRDataService struct {
	client *tws.Client
}

// Ensure IBKRDataService implements interfaces.DataService
var _ interfaces.DataService = (*IBKRDataService)(nil)

func NewIBKRDataService(client *tws.Client) *IBKRDataService {
	return &IBKRDataService{client: client}
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
	// Simplified mapping for stocks
	contract := tws.Contract{
		Symbol:   symbol,
		SecType:  tws.Stock,
		Exchange: "SMART",
		Currency: "USD",
	}

	reqId := s.client.NextOrderId()
	ch := s.client.Register(reqId)
	defer s.client.Complete(reqId)

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
	contract := tws.Contract{
		Symbol:   symbol,
		SecType:  tws.Stock,
		Exchange: "SMART",
		Currency: "USD",
	}

	reqId := s.client.NextOrderId()
	ch := s.client.Register(reqId)
	defer s.client.Complete(reqId)

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
