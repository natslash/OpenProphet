package services

import (
	"context"
	"fmt"
	"prophet-trader/interfaces"
	"prophet-trader/tws"
	"time"

	"github.com/sirupsen/logrus"
)

// IbkrDataService implements interfaces.DataService using IBKR TWS API
type IbkrDataService struct {
	client *tws.Client
	logger *logrus.Logger
}

// NewIbkrDataService creates a new IBKR data service
func NewIbkrDataService(client *tws.Client) *IbkrDataService {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	return &IbkrDataService{
		client: client,
		logger: logger,
	}
}

// Ensure IbkrDataService implements interfaces.DataService
var _ interfaces.DataService = (*IbkrDataService)(nil)

// GetHistoricalBars retrieves historical bar data
func (s *IbkrDataService) GetHistoricalBars(ctx context.Context, symbol string, start, end time.Time, timeframe string) ([]*interfaces.Bar, error) {
	s.logger.WithFields(logrus.Fields{
		"symbol":    symbol,
		"start":     start,
		"end":       end,
		"timeframe": timeframe,
	}).Info("Fetching historical bars from IBKR")

	// 1. Resolve symbol to Contract
	contract := tws.Contract{
		Symbol:   symbol,
		SecType:  tws.Stock,
		Exchange: "SMART",
		Currency: "USD",
	}
	details, err := s.client.ReqContractDetails(ctx, contract)
	if err != nil || len(details) == 0 {
		return nil, fmt.Errorf("failed to resolve contract for %s: %v", symbol, err)
	}
	contract = details[0].Contract

	// 2. reqHistoricalData
	reqID := s.client.NextOrderId()
	ch := s.client.Dispatcher().Register(reqID)
	defer s.client.Dispatcher().Complete(reqID)

	// Format for IBKR: "yyyyMMdd HH:mm:ss"
	endStr := end.Format("20060102 15:04:05")
	duration := fmt.Sprintf("%d D", int(end.Sub(start).Hours()/24)+1)
	barSize := "1 day" // Simplified mapping

	// TWS historical data response terminates with an empty HistoricalData message
	// with ReqId > 0, Date == "", and Count == -1
	err = s.client.Encoder().ReqHistoricalData(reqID, contract, endStr, duration, barSize, "TRADES", 1, 1, false)
	if err != nil {
		return nil, fmt.Errorf("failed to request historical data: %w", err)
	}

	// 3. Wait for historical data response
	var bars []*interfaces.Bar
	var lastErr error
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return bars, lastErr
			}
			switch m := msg.(type) {
			case tws.HistoricalData:
				if m.Count == -1 { // Termination marker
					return bars, nil
				}
				// Attempt to parse Date: "20060102 15:04:05"
				t, err := time.Parse("20060102 15:04:05", m.Date)
				if err != nil {
					t = time.Now()
				}
				bars = append(bars, &interfaces.Bar{
					Symbol:    symbol,
					Timestamp: t,
					Open:      m.Open,
					High:      m.High,
					Low:       m.Low,
					Close:     m.Close,
					Volume:    int64(m.Volume),
				})
			case error:
				// If we get an error, check if it's a rate limit error (e.g., code 162)
				// For now, keep it simple and return the error
				lastErr = m
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Second): // Extended timeout
			return bars, fmt.Errorf("timeout waiting for historical data")
		}
	}
}

// GetLatestBar retrieves the most recent bar for a symbol
func (s *IbkrDataService) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	return nil, fmt.Errorf("GetLatestBar not implemented for IBKR yet")
}

// GetLatestQuote retrieves the most recent quote for a symbol
func (s *IbkrDataService) GetLatestQuote(ctx context.Context, symbol string) (*interfaces.Quote, error) {
	s.logger.WithField("symbol", symbol).Info("Fetching latest quote from IBKR")

	// 1. Resolve symbol to Contract (Assume stock for now)
	contract := tws.Contract{
		Symbol:   symbol,
		SecType:  tws.Stock,
		Exchange: "SMART",
		Currency: "USD",
	}

	details, err := s.client.ReqContractDetails(ctx, contract)
	if err != nil || len(details) == 0 {
		return nil, fmt.Errorf("failed to resolve contract for %s: %v", symbol, err)
	}
	contract = details[0].Contract

	// 2. reqMktData(contract)
	reqID := s.client.NextOrderId()
	ch := s.client.Dispatcher().Register(reqID)
	defer s.client.Dispatcher().Complete(reqID)

	if err := s.client.Encoder().ReqMktData(reqID, contract, "", true, false); err != nil {
		return nil, fmt.Errorf("failed to request market data: %w", err)
	}

	// 3. Wait for tick/quote response
	var bidPrice, askPrice float64
	var bidSize, askSize int64
	var receivedCount int

	for {
		select {
		case msg := <-ch:
			switch m := msg.(type) {
			case tws.TickPriceMsg:
				if m.TickType == 1 { // Bid
					bidPrice = m.Price
				} else if m.TickType == 2 { // Ask
					askPrice = m.Price
				}
				receivedCount++
			case tws.TickSizeMsg:
				if m.TickType == 0 { // Bid Size
					bidSize = m.Size.IntPart()
				} else if m.TickType == 3 { // Ask Size
					askSize = m.Size.IntPart()
				}
				receivedCount++
			case error:
				return nil, m
			}
			if receivedCount >= 4 { // Got enough for a basic quote
				return &interfaces.Quote{
					Symbol:    symbol,
					BidPrice:  bidPrice,
					BidSize:   bidSize,
					AskPrice:  askPrice,
					AskSize:   askSize,
					Timestamp: time.Now(),
				}, nil
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// GetLatestTrade retrieves the most recent trade for a symbol
func (s *IbkrDataService) GetLatestTrade(ctx context.Context, symbol string) (*interfaces.Trade, error) {
	return nil, fmt.Errorf("GetLatestTrade not implemented for IBKR yet")
}

// StreamBars starts streaming bar data for specified symbols
func (s *IbkrDataService) StreamBars(ctx context.Context, symbols []string) (<-chan *interfaces.Bar, error) {
	barChan := make(chan *interfaces.Bar)

	s.logger.WithField("symbols", symbols).Info("Streaming bars not implemented for IBKR yet")

	go func() {
		defer close(barChan)
		<-ctx.Done()
	}()

	return barChan, nil
}
