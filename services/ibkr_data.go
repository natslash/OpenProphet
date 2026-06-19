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

func (s *IBKRDataService) HistoricalData(reqId int64, bar tws.HistoricalBar) {
	s.mu.RLock()
	ch, ok := s.streams[reqId]
	s.mu.RUnlock()
	if ok {
		// Blocking send for historical data to avoid dropping bars
		ch <- tws.HistoricalDataMsg{ReqId: reqId, Bar: bar}
	}
}

func (s *IBKRDataService) HistoricalDataEnd(reqId int64, startDateStr, endDateStr string) {
	s.mu.RLock()
	ch, ok := s.streams[reqId]
	s.mu.RUnlock()
	if ok {
		// Blocking send for end message
		ch <- tws.HistoricalDataEndMsg{ReqId: reqId, Start: startDateStr, End: endDateStr}
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
	
	days := int(diff.Hours() / 24)
	if days <= 0 {
		days = 1
	}

	// For intraday bars, max duration in days is limited.
	// But simply specifying 'D' usually works for both intraday and daily bars.
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
	switch secType {
	case tws.Option:
		return "MIDPOINT" // Options trades can be sparse
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

	var bars []*interfaces.Bar

	for {
		select {
		case msg := <-ch:
			switch t := msg.(type) {
			case tws.HistoricalDataMsg:
				epochStr := t.Bar.Date
				// If formatDate=2, date is epoch seconds string
				// If the server doesn't honor it or it's a daily bar, it might be "20260618".
				// Let's parse appropriately.
				var ts time.Time
				if len(epochStr) == 8 {
					// YYYYMMDD fallback
					if parsed, err := time.Parse("20060102", epochStr); err == nil {
						ts = parsed
					}
				} else {
					if epochSecs, err := strconv.ParseInt(epochStr, 10, 64); err == nil {
						ts = time.Unix(epochSecs, 0)
					} else {
						// Last resort fallback
						ts = time.Now()
					}
				}

				bars = append(bars, &interfaces.Bar{
					Symbol:    symbol,
					Timestamp: ts,
					Open:      t.Bar.Open,
					High:      t.Bar.High,
					Low:       t.Bar.Low,
					Close:     t.Bar.Close,
					Volume:    t.Bar.Volume.IntPart(),
					VWAP:      t.Bar.WAP.InexactFloat64(),
				})

			case tws.HistoricalDataEndMsg:
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
	// For latest bar, request last 1 day of 1-minute bars (or requested timeframe, but interface doesn't pass timeframe)
	// We'll hardcode to "1 min" for now or use GetHistoricalBars
	bars, err := s.GetHistoricalBars(ctx, symbol, time.Now().Add(-24*time.Hour), time.Now(), "1Min")
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
