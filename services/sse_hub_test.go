package services

import (
	"testing"
	"time"
)

// TestSSEHub_UnsubscribeDoesNotBlock guards against the goroutine-leak
// regression: Unsubscribe must return promptly even with buffered events
// (the old drain loop blocked forever on the never-closed channel).
func TestSSEHub_UnsubscribeDoesNotBlock(t *testing.T) {
	h := NewSSEHub()
	ch := h.Subscribe()
	for i := 0; i < 5; i++ {
		h.Broadcast("tick", map[string]int{"n": i})
	}

	done := make(chan struct{})
	go func() { h.Unsubscribe(ch); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Unsubscribe blocked — goroutine leak")
	}
	if h.ClientCount() != 0 {
		t.Errorf("client not removed: count=%d", h.ClientCount())
	}
}

// TestSSEHub_BroadcastNonBlocking confirms a full/slow client never blocks the
// broadcaster (events are dropped, not parked).
func TestSSEHub_BroadcastNonBlocking(t *testing.T) {
	h := NewSSEHub()
	_ = h.Subscribe() // never drained → buffer fills

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Broadcast("flood", map[string]int{"n": i})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Broadcast blocked on a full client buffer")
	}
}
