package services

import (
	"encoding/json"
	"sync"
)

type SSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

var Hub *SSEHub

func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

func (h *SSEHub) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *SSEHub) Unsubscribe(ch chan SSEEvent) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	// Drain remaining events so senders don't block.
	for range ch {
	}
}

func (h *SSEHub) Broadcast(event string, data interface{}) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := SSEEvent{Event: event, Data: raw}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// Client too slow — drop event rather than blocking the broadcaster.
		}
	}
}

func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
