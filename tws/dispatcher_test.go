package tws

import (
	"sync"
	"testing"
	"time"
)

func TestDispatcher_RegisterAndDispatch(t *testing.T) {
	d := NewDispatcher()

	ch1 := d.Register(100)
	ch2 := d.Register(200)

	// Dispatch to 100
	go func() {
		time.Sleep(10 * time.Millisecond)
		d.Dispatch(100, "hello 100")
	}()

	// Dispatch to 200
	go func() {
		time.Sleep(20 * time.Millisecond)
		d.Dispatch(200, "hello 200")
	}()

	select {
	case msg := <-ch1:
		if msg != "hello 100" {
			t.Errorf("Expected 'hello 100', got %v", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for reqId 100")
	}

	select {
	case msg := <-ch2:
		if msg != "hello 200" {
			t.Errorf("Expected 'hello 200', got %v", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for reqId 200")
	}

	d.Complete(100)
	d.Complete(200)

	d.mu.RLock()
	if len(d.pending) != 0 {
		t.Errorf("Expected pending map to be empty, got %d items", len(d.pending))
	}
	d.mu.RUnlock()
}

func TestDispatcher_DispatchUnregistered(t *testing.T) {
	d := NewDispatcher()
	// Should not panic or block
	d.Dispatch(999, "nobody listening")
}

func TestDispatcher_Concurrency(t *testing.T) {
	d := NewDispatcher()
	var wg sync.WaitGroup

	// Hammer the dispatcher concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(reqId int64) {
			defer wg.Done()
			ch := d.Register(reqId)

			go func() {
				d.Dispatch(reqId, reqId*2)
			}()

			select {
			case msg := <-ch:
				val := msg.(int64)
				if val != reqId*2 {
					t.Errorf("Expected %d, got %d", reqId*2, val)
				}
			case <-time.After(500 * time.Millisecond):
				t.Errorf("Timeout on concurrent dispatch for %d", reqId)
			}
			d.Complete(reqId)
		}(int64(i))
	}

	wg.Wait()
}
