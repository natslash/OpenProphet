package tws

import (
	"sync"
	"testing"
)

func TestOrderIdManager_Sequential(t *testing.T) {
	m := &OrderIdManager{}
	m.Seed(100)

	if id := m.Next(); id != 100 {
		t.Errorf("Expected 100, got %d", id)
	}
	if id := m.Next(); id != 101 {
		t.Errorf("Expected 101, got %d", id)
	}
	if id := m.Next(); id != 102 {
		t.Errorf("Expected 102, got %d", id)
	}
}

func TestOrderIdManager_Concurrency(t *testing.T) {
	m := &OrderIdManager{}
	m.Seed(1)

	var wg sync.WaitGroup
	const numGoroutines = 100
	const incrementsPerRoutine = 100

	results := make(chan int64, numGoroutines*incrementsPerRoutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerRoutine; j++ {
				results <- m.Next()
			}
		}()
	}

	wg.Wait()
	close(results)

	seen := make(map[int64]bool)
	for id := range results {
		if seen[id] {
			t.Errorf("Duplicate order ID detected: %d", id)
		}
		seen[id] = true
	}

	expectedTotal := numGoroutines * incrementsPerRoutine
	if len(seen) != expectedTotal {
		t.Errorf("Expected %d unique IDs, got %d", expectedTotal, len(seen))
	}
}
