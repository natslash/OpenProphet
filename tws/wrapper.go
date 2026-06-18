package tws

// Wrapper is the callback interface for receiving decoded messages from TWS.
// It represents the Go equivalent of the Java EWrapper interface.
type Wrapper interface {
	NextValidId(orderId int64)
	ManagedAccounts(accountsList string)
	Error(reqId int, code int, msg string)
	CurrentTime(timeInSeconds int64)
	ContractDetails(reqId int64, details ContractDetails)
	ContractDetailsEnd(reqId int64)
}
