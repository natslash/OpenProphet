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
	client   *tws.Client
	resolver *tws.ContractResolver
	mu       sync.RWMutex
	streams  map[int64]chan any
	histBuf  map[int64][]*interfaces.Bar
}

// Ensure IBKRDataService implements interfaces.DataService
var _ interfaces.DataService = (*IBKRDataService)(nil)

func NewIBKRDataService(client *tws.Client, resolver *tws.ContractResolver) *IBKRDataService {
	s := &IBKRDataService{
		client:   client,
		resolver: resolver,
		streams:  make(map[int64]chan any),
		histBuf:  make(map[int64][]*interfaces.Bar),
	}
	client.AddWrapper(s)
	// Type 4 = delayed-frozen: use live data when available, fall back to
	// delayed/frozen snapshots when not (e.g. outside market hours or without
	// a live subscription for the instrument).
	_ = client.Encoder().ReqMarketDataType(4)
	return s
}

// resolveContract resolves an interface Symbol via the ContractResolver,
// which tries the hardcoded symbology first and falls back to IBKR lookup.
func (s *IBKRDataService) resolveContract(ctx context.Context, symbol string) (tws.Contract, error) {
	return s.resolver.Resolve(ctx, symbol)
}

// OnDisconnect cleans up stale state when IB Gateway drops. Closes all
// open streaming channels and clears historical buffers.
func (s *IBKRDataService) OnDisconnect() {
	s.mu.Lock()
	for id, ch := range s.streams {
		close(ch)
		delete(s.streams, id)
	}
	for id := range s.histBuf {
		delete(s.histBuf, id)
	}
	s.mu.Unlock()
}

// OnReconnect re-initialises state after a successful reconnect.
func (s *IBKRDataService) OnReconnect() {
	_ = s.client.Encoder().ReqMarketDataType(4)
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

func (s *IBKRDataService) TickOptionComputation(reqId int64, tickType int, tickAttrib int, impliedVol, delta, optPrice, pvDividend, gamma, vega, theta, undPrice float64) {
	s.mu.RLock()
	ch, ok := s.streams[reqId]
	s.mu.RUnlock()
	if ok {
		select {
		case ch <- tws.TickOptionComputationMsg{
			TickType: tickType, TickAttrib: tickAttrib,
			ImpliedVol: impliedVol, Delta: delta, OptPrice: optPrice,
			PvDividend: pvDividend, Gamma: gamma, Vega: vega,
			Theta: theta, UndPrice: undPrice,
		}:
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
	if diff <= 0 {
		return "1 D" // defensive default
	}

	intraday := strings.HasSuffix(barSize, "min") || strings.HasSuffix(barSize, "mins") ||
		strings.HasSuffix(barSize, "hour") || strings.HasSuffix(barSize, "hours")

	// Sub-day windows for intraday bars: request the exact number of seconds
	// instead of rounding up to a full day (avoids over-fetching). IBKR's "S"
	// duration unit tops out at 86400, which a sub-day window never exceeds.
	if intraday && diff < 24*time.Hour {
		secs := int(diff.Seconds())
		if secs < 60 {
			secs = 60 // ask for at least one minute of data
		}
		return fmt.Sprintf("%d S", secs)
	}

	days := int(diff.Hours() / 24)
	if days < 1 {
		days = 1
	}

	// IBKR caps how much intraday data a single request may span; clamp to
	// avoid 162/322 errors (1-min is the tightest).
	if strings.HasSuffix(barSize, "min") || strings.HasSuffix(barSize, "mins") {
		cap := 5
		if barSize == "1 min" || barSize == "2 mins" {
			cap = 2
		}
		if days > cap {
			days = cap
		}
		return fmt.Sprintf("%d D", days)
	}

	// Daily and larger bar sizes.
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
	contract, err := s.resolveContract(ctx, symbol)
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
	contract, err := s.resolveContract(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("GetLatestQuote: %w", err)
	}

	isIndex := contract.SecType == tws.Index

	reqId := s.client.NextOrderId()
	ch := s.subscribe(reqId)
	defer s.unsubscribe(reqId)

	if err := s.client.Encoder().ReqMktData(reqId, contract, "", false, false); err != nil {
		return nil, fmt.Errorf("ReqMktData error: %w", err)
	}
	defer s.client.Encoder().CancelMktData(reqId)

	quote := &interfaces.Quote{
		Symbol:    symbol,
		Timestamp: time.Now(),
	}

	var hasBid, hasAsk, hasLast bool
	deadline := time.After(15 * time.Second)

	for {
		if isIndex && hasLast {
			return quote, nil
		}
		if !isIndex && hasBid && hasAsk {
			return quote, nil
		}

		select {
		case msg := <-ch:
			switch t := msg.(type) {
			case tws.TickPriceMsg:
				switch t.TickType {
				case tws.TickBidPrice, tws.TickDelayedBid:
					quote.BidPrice = t.Price
					if !t.Size.IsZero() {
						quote.BidSize = t.Size.IntPart()
					}
					hasBid = true
				case tws.TickAskPrice, tws.TickDelayedAsk:
					quote.AskPrice = t.Price
					if !t.Size.IsZero() {
						quote.AskSize = t.Size.IntPart()
					}
					hasAsk = true
				case tws.TickLastPrice, tws.TickDelayedLast:
					if isIndex && t.Price > 0 {
						quote.BidPrice = t.Price
						quote.AskPrice = t.Price
						hasLast = true
					}
				case tws.TickClose, tws.TickDelayedClose:
					if isIndex && !hasLast && t.Price > 0 {
						quote.BidPrice = t.Price
						quote.AskPrice = t.Price
						hasLast = true
					}
				}
			case tws.TickSizeMsg:
				switch t.TickType {
				case tws.TickBidSize, tws.TickDelayedBidSize:
					quote.BidSize = t.Size.IntPart()
				case tws.TickAskSize, tws.TickDelayedAskSize:
					quote.AskSize = t.Size.IntPart()
				}
			case error:
				return nil, fmt.Errorf("tws error: %w", t)
			}
		case <-deadline:
			if quote.BidPrice > 0 || quote.AskPrice > 0 {
				return quote, nil
			}
			return nil, fmt.Errorf("quote timeout: no ticks received within 15s for %s", symbol)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *IBKRDataService) GetLatestTrade(ctx context.Context, symbol string) (*interfaces.Trade, error) {
	contract, err := s.resolveContract(ctx, symbol)
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
				switch t.TickType {
				case tws.TickLastPrice, tws.TickDelayedLast:
					trade.Price = t.Price
					if !t.Size.IsZero() {
						trade.Size = t.Size.IntPart()
						hasSize = true
					}
					hasPrice = true
				}
			case tws.TickSizeMsg:
				switch t.TickType {
				case tws.TickLastSize, tws.TickDelayedLastSize:
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
