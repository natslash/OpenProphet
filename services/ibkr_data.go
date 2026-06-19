package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type IBKRDataService struct {
	tws.DefaultWrapper
	client  *tws.Client
	mu      sync.RWMutex
	streams map[int64]chan any
	histBuf map[int64][]*interfaces.Bar
}

// Ensure IBKRDataService implements interfaces.DataService
var _ interfaces.DataService = (*IBKRDataService)(nil)

func NewIBKRDataService(client *tws.Client) *IBKRDataService {
	s := &IBKRDataService{
		client:  client,
		streams: make(map[int64]chan any),
		histBuf: make(map[int64][]*interfaces.Bar),
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

func (s *IBKRDataService) HistoricalData(reqId int64, bar tws.HistoricalBar) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only buffer if we actually have an active subscription for this reqId
	if _, ok := s.streams[reqId]; ok {
		s.histBuf[reqId] = append(s.histBuf[reqId], &interfaces.Bar{
			Timestamp: tws.ParseTWSDate(bar.Date),
			Open:      bar.Open,
			High:      bar.High,
			Low:       bar.Low,
			Close:     bar.Close,
			Volume:    bar.Volume.IntPart(),
			VWAP:      bar.WAP.InexactFloat64(),
		})
	}
}

func (s *IBKRDataService) HistoricalDataEnd(reqId int64, startDateStr, endDateStr string) {
	s.mu.Lock()
	ch, ok := s.streams[reqId]
	bars := s.histBuf[reqId]
	delete(s.histBuf, reqId)
	s.mu.Unlock()

	if ok {
		// Non-blocking handoff of the entire assembled slice to avoid select/default dropping
		ch <- tws.HistoricalDataEndMsg{ReqId: reqId, Start: startDateStr, End: endDateStr, ExtData: bars}
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
	delete(s.histBuf, reqId)
	s.mu.Unlock()
}

func mapTimeframeToIBKR(tf string) string {
	switch tf {
	case "1Min":
		return "1 min"
	case "5Min":
		return "5 mins"
	case "15Min":
		return "15 mins"
	case "30Min":
		return "30 mins"
	case "1Hour":
		return "1 hour"
	case "4Hour":
		return "4 hours"
	case "1Day":
		return "1 day"
	default:
		return "1 hour" // fallback
	}
}

func calculateDuration(start, end time.Time, barSize string) string {
	diff := end.Sub(start)
	if diff < 0 {
		diff = 0
	}
	
	// IBKR imposes strict limits on intraday historical data queries.
	// Cap durations aggressively to avoid 162/322 errors.
	if strings.HasSuffix(barSize, "min") || strings.HasSuffix(barSize, "mins") {
		days := int(diff.Hours() / 24)
		if days < 1 {
			days = 1
		}
		// Max 2 days for 1-minute bars, safely map 5-min to max 5 days.
		cap := 5
		if barSize == "1 min" || barSize == "2 mins" {
			cap = 2
		}
		if days > cap {
			days = cap
		}
		return fmt.Sprintf("%d D", days)
	}

	// Sub-day durations format as "X S" (seconds) to prevent over-fetching
	hours := diff.Hours()
	if hours < 24 && hours > 0 {
		return fmt.Sprintf("%d S", int(diff.Seconds()))
	}

	days := int(hours / 24)
	if days <= 0 {
		days = 1
	}

	// Simply specifying 'D' works for daily bars.
	if days > 365 {
		years := days / 365
		if days%365 > 0 {
			years++
		}
		return fmt.Sprintf("%d Y", years)
	}

	return fmt.Sprintf("%d D", days)
}

func getWhatToShow(secType tws.InstrumentType) string {
	// Options trade sparsely, so TRADES yields flat/gappy bars with ~zero
	// volume; MIDPOINT gives a clean continuous price series for analysis.
	// Stocks/futures/indices use TRADES for real OHLCV (incl. volume).
	switch secType {
	case tws.Option:
		return "MIDPOINT"
	default:
		return "TRADES"
	}
}

func (s *IBKRDataService) GetHistoricalBars(ctx context.Context, symbol string, start, end time.Time, timeframe string) ([]*interfaces.Bar, error) {
	contract, err := symbolToContract(symbol)
	if err != nil {
		return nil, fmt.Errorf("GetHistoricalBars: %w", err)
	}

	reqId := s.client.NextOrderId()
	ch := s.subscribe(reqId)
	defer s.unsubscribe(reqId)

	barSize := mapTimeframeToIBKR(timeframe)
	duration := calculateDuration(start, end, barSize)
	whatToShow := getWhatToShow(contract.SecType)

	// Format end date for TWS (yyyyMMdd-HH:mm:ss in UTC)
	endDateTime := end.UTC().Format("20060102-15:04:05")

	// formatDate=2 to return epoch seconds instead of string dates
	err = s.client.Encoder().ReqHistoricalData(s.client.ServerVersion(), reqId, contract, endDateTime, duration, barSize, whatToShow, 1, 2, false)
	if err != nil {
		return nil, fmt.Errorf("ReqHistoricalData error: %w", err)
	}

	for {
		select {
		case msg := <-ch:
			switch t := msg.(type) {
			case tws.HistoricalDataEndMsg:
				bars, _ := t.ExtData.([]*interfaces.Bar)
				for _, b := range bars {
					b.Symbol = symbol
				}
				return bars, nil

			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRDataService) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	// 5-day lookback guarantees we cover 3-day weekends + market holidays.
	// Using 5Min allows up to 5 days without hitting the IBKR intraday duration caps.
	bars, err := s.GetHistoricalBars(ctx, symbol, time.Now().Add(-5*24*time.Hour), time.Now(), "5Min")
	if err != nil {
		return nil, err
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no bars found")
	}
	return bars[len(bars)-1], nil
}

func (s *IBKRDataService) StreamBars(ctx context.Context, symbols []string) (<-chan *interfaces.Bar, error) {
	return nil, fmt.Errorf("StreamBars not implemented in Phase 4")
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
