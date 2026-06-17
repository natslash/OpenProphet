package tws

import "sync"

// Dispatcher manages the routing of asynchronous TWS responses
// to the waiting callers using a reqId -> channel registry.
type Dispatcher struct {
	mu      sync.RWMutex
	pending map[int64]chan any
}

// NewDispatcher creates a new dispatcher instance.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		pending: make(map[int64]chan any),
	}
}

// Register creates a new buffered channel for the given reqId, stores it,
// and returns it to the caller to wait on.
func (d *Dispatcher) Register(reqId int64) <-chan any {
	ch := make(chan any, 16) // Buffered to prevent blocking the decoder
	d.mu.Lock()
	d.pending[reqId] = ch
	d.mu.Unlock()
	return ch
}

// Dispatch sends a message to the channel registered under reqId.
func (d *Dispatcher) Dispatch(reqId int64, msg any) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	ch, ok := d.pending[reqId]
	if ok {
		select {
		case ch <- msg:
		default:
			// Buffer full, drop or log
		}
	}
}

// Complete closes the channel associated with reqId and removes it from the registry.
func (d *Dispatcher) Complete(reqId int64) {
	d.mu.Lock()
	if ch, ok := d.pending[reqId]; ok {
		close(ch)
		delete(d.pending, reqId)
	}
	d.mu.Unlock()
}
