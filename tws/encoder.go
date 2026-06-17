package tws

import "strconv"

// FieldWriter represents a transport that can send length-prefixed,
// null-delimited TWS messages.
type FieldWriter interface {
	SendFields(fields ...string) error
}

// Encoder translates typed request methods into TWS wire format messages.
type Encoder struct {
	writer FieldWriter
}

// NewEncoder creates a new protocol encoder.
func NewEncoder(w FieldWriter) *Encoder {
	return &Encoder{writer: w}
}

// ReqCurrentTime requests the current system time on the server side.
// The TWS server will respond with an inCurrentTime message containing the epoch time.
func (e *Encoder) ReqCurrentTime() error {
	const version = "1"
	return e.writer.SendFields(strconv.Itoa(outReqCurrentTime), version)
}
