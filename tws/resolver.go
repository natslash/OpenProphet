package tws

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ContractResolver struct {
	client *Client
	mu     sync.RWMutex
	cache  map[string]Contract
}

func NewContractResolver(client *Client) *ContractResolver {
	return &ContractResolver{
		client: client,
		cache:  make(map[string]Contract),
	}
}

func (r *ContractResolver) Resolve(ctx context.Context, symbol string) (Contract, error) {
	r.mu.RLock()
	if c, ok := r.cache[symbol]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	if c, err := ParseSymbol(symbol); err == nil {
		r.put(symbol, c)
		return c, nil
	}

	c, err := r.resolveViaIBKR(ctx, symbol)
	if err != nil {
		return Contract{}, err
	}
	r.put(symbol, c)
	return c, nil
}

func (r *ContractResolver) resolveViaIBKR(ctx context.Context, symbol string) (Contract, error) {
	partial := Contract{
		Symbol:   symbol,
		SecType:  Stock,
		Exchange: "SMART",
		Currency: "USD",
	}
	results, err := r.client.ReqContractDetails(ctx, partial)
	if err == nil && len(results) > 0 {
		return r.pickBest(results), nil
	}

	partial.Currency = "EUR"
	results, err = r.client.ReqContractDetails(ctx, partial)
	if err == nil && len(results) > 0 {
		return r.pickBest(results), nil
	}

	partial = Contract{Symbol: symbol, SecType: Index, Exchange: "SMART"}
	results, err = r.client.ReqContractDetails(ctx, partial)
	if err == nil && len(results) > 0 {
		return r.pickBest(results), nil
	}

	return Contract{}, fmt.Errorf("no contract found for %q", symbol)
}

func (r *ContractResolver) pickBest(results []ContractDetails) Contract {
	if len(results) == 1 {
		return results[0].Contract
	}
	for _, cd := range results {
		if cd.Contract.PrimaryExch != "" {
			return cd.Contract
		}
	}
	return results[0].Contract
}

func (r *ContractResolver) put(symbol string, c Contract) {
	r.mu.Lock()
	r.cache[symbol] = c
	r.mu.Unlock()
}

func (r *ContractResolver) Format(c Contract) string {
	return FormatSymbol(c)
}

func (r *ContractResolver) Search(ctx context.Context, symbol string, secType string, exchange string, currency string) ([]ContractDetails, error) {
	partial := Contract{Symbol: symbol}
	if secType != "" {
		partial.SecType = InstrumentType(strings.ToUpper(secType))
	}
	if exchange != "" {
		partial.Exchange = exchange
	}
	if currency != "" {
		partial.Currency = strings.ToUpper(currency)
	}
	return r.client.ReqContractDetails(ctx, partial)
}

func (r *ContractResolver) ClearCache() {
	r.mu.Lock()
	r.cache = make(map[string]Contract)
	r.mu.Unlock()
}
