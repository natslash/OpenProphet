package tws

import "sync/atomic"

// OrderIdManager provides thread-safe access to a monotonically
// increasing order ID counter, seeded from the TWS handshake.
type OrderIdManager struct {
	nextId atomic.Int64
}

// Seed initializes the manager with the first valid order ID from TWS.
func (m *OrderIdManager) Seed(id int64) {
	m.nextId.Store(id)
}

// Next atomically increments the counter and returns a valid order ID.
func (m *OrderIdManager) Next() int64 {
	// Add returns the new value, so we subtract 1 to get the value *before* increment.
	return m.nextId.Add(1) - 1
}
