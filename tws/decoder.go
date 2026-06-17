package tws

import (
	"fmt"
	"strconv"
)

// Decoder parses incoming \0-delimited TWS messages and dispatches them
// to the appropriate methods on the Wrapper interface.
type Decoder struct {
	wrapper Wrapper
}

// NewDecoder creates a new protocol decoder.
func NewDecoder(w Wrapper) *Decoder {
	return &Decoder{wrapper: w}
}

// Decode processes a single incoming TWS message payload.
func (d *Decoder) Decode(fields []string) error {
	if len(fields) == 0 || fields[0] == "" {
		return nil
	}

	msgID, err := strconv.Atoi(fields[0])
	if err != nil {
		return fmt.Errorf("invalid message ID %q: %w", fields[0], err)
	}

	switch msgID {
	case inNextValidID: // [9, version, orderId]
		if len(fields) >= 3 {
			if orderID, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.NextValidId(orderID)
			}
		}

	case inManagedAccts: // [15, version, accountsCSV]
		if len(fields) >= 3 {
			d.wrapper.ManagedAccounts(fields[2])
		}

	case inErrMsg: // [4, version, id, code, msg, ...]
		if len(fields) >= 5 {
			reqID, _ := strconv.Atoi(fields[2])
			code, _ := strconv.Atoi(fields[3])
			d.wrapper.Error(reqID, code, fields[4])
		}

	case inCurrentTime: // [49, version, time]
		if len(fields) >= 3 {
			if t, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				d.wrapper.CurrentTime(t)
			}
		}

	default:
		// Unhandled message type in this phase
	}

	return nil
}
