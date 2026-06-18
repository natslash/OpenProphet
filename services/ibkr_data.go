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

	// TODO: Implement IBKR historical data request
	return nil, fmt.Errorf("GetHistoricalBars not implemented for IBKR yet")
}

// GetLatestBar retrieves the most recent bar for a symbol
func (s *IbkrDataService) GetLatestBar(ctx context.Context, symbol string) (*interfaces.Bar, error) {
	return nil, fmt.Errorf("GetLatestBar not implemented for IBKR yet")
}

// GetLatestQuote retrieves the most recent quote for a symbol
func (s *IbkrDataService) GetLatestQuote(ctx context.Context, symbol string) (*interfaces.Quote, error) {
	s.logger.WithField("symbol", symbol).Info("Fetching latest quote from IBKR")
	
	// IBKR-specific implementation logic will go here
	// 1. Resolve symbol to Contract
	// 2. reqMktData(contract)
	// 3. Wait for tick/quote response
	
	return nil, fmt.Errorf("GetLatestQuote not implemented for IBKR yet")
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
