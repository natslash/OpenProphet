package tws

import "github.com/shopspring/decimal"

// DefaultWrapper is a no-op implementation of Wrapper. Embed this in custom wrappers
// so you only have to implement the methods you care about without panicking.
type DefaultWrapper struct{}

func (w *DefaultWrapper) NextValidId(orderId int64) {}
func (w *DefaultWrapper) ManagedAccounts(accountsList string) {}
func (w *DefaultWrapper) Error(reqId int, code int, msg string) {}
func (w *DefaultWrapper) CurrentTime(timeInSeconds int64) {}
func (w *DefaultWrapper) ContractDetails(reqId int64, details ContractDetails) {}
func (w *DefaultWrapper) ContractDetailsEnd(reqId int64) {}
func (w *DefaultWrapper) TickPrice(reqId int64, tickType int, price float64, size decimal.Decimal, attr TickAttrib) {}
func (w *DefaultWrapper) TickSize(reqId int64, tickType int, size decimal.Decimal) {}
func (w *DefaultWrapper) AccountSummary(reqId int64, account, tag, value, currency string) {}
func (w *DefaultWrapper) AccountSummaryEnd(reqId int64) {}
func (w *DefaultWrapper) Position(account string, contract Contract, position decimal.Decimal, avgCost float64) {}
func (w *DefaultWrapper) PositionEnd() {}
func (w *DefaultWrapper) OpenOrder(orderId int64, contract Contract, order Order, orderState OrderState) {}
func (w *DefaultWrapper) OpenOrderEnd() {}
func (w *DefaultWrapper) OrderStatus(orderId int64, status string, filled, remaining decimal.Decimal, avgFillPrice float64, permId, parentId int64, lastFillPrice float64, clientId int, whyHeld string, mktCapPrice float64) {}
