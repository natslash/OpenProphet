package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestIntentManager(ttlSeconds int) *IntentManager {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel) // keep test output quiet
	return NewIntentManager(ttlSeconds, logger)
}

func TestIntentManager_CreateGetList(t *testing.T) {
	im := newTestIntentManager(300)

	id, err := im.CreateIntent(IntentTypeManagedPosition, []byte(`{}`), "ESTX50", "buy", 1, 5000)
	if err != nil {
		t.Fatalf("CreateIntent: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty intent id")
	}

	got, err := im.GetIntent(id)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.Status != IntentStatusPending {
		t.Errorf("status = %s, want PENDING", got.Status)
	}
	if got.Symbol != "ESTX50" || got.Side != "buy" || got.Quantity != 1 {
		t.Errorf("intent fields wrong: %+v", got)
	}

	if n := len(im.ListIntents()); n != 1 {
		t.Errorf("ListIntents len = %d, want 1", n)
	}

	if _, err := im.GetIntent("does-not-exist"); err == nil {
		t.Error("GetIntent for missing id should error")
	}
}

func TestIntentManager_ClaimForExecution_Atomic(t *testing.T) {
	im := newTestIntentManager(300)
	id, _ := im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "sell", 1, 5000)

	// First claim succeeds and flips status to EXECUTING.
	intent, err := im.ClaimForExecution(id)
	if err != nil {
		t.Fatalf("first ClaimForExecution: %v", err)
	}
	if intent.Status != IntentStatusExecuting {
		t.Errorf("status = %s, want EXECUTING", intent.Status)
	}

	// Second claim must fail — the intent is no longer pending. This is what
	// prevents a double-fire from two concurrent authorize calls.
	if _, err := im.ClaimForExecution(id); err == nil {
		t.Error("second ClaimForExecution should fail (intent not pending)")
	}

	// Claiming a missing intent fails.
	if _, err := im.ClaimForExecution("nope"); err == nil {
		t.Error("ClaimForExecution for missing id should fail")
	}
}

func TestIntentManager_ClaimForExecution_OnlyOneWinnerUnderRace(t *testing.T) {
	im := newTestIntentManager(300)
	id, _ := im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "sell", 1, 5000)

	const goroutines = 50
	var wins int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := im.ClaimForExecution(id); err == nil {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Errorf("exactly one claim should win, got %d", wins)
	}
}

func TestIntentManager_RejectIntent_FiresFeedbackAndRemoves(t *testing.T) {
	im := newTestIntentManager(300)

	var cbCount int64
	var gotReason string
	var mu sync.Mutex
	done := make(chan struct{}, 1)
	im.SetFeedbackCallback(func(intent *Intent, reason string) {
		mu.Lock()
		gotReason = reason
		mu.Unlock()
		atomic.AddInt64(&cbCount, 1)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	id, _ := im.CreateIntent(IntentTypeManagedPosition, []byte(`{}`), "ESTX50", "buy", 1, 5000)
	if err := im.RejectIntent(id, "Rejected by user"); err != nil {
		t.Fatalf("RejectIntent: %v", err)
	}

	// Intent must be gone from the queue.
	if _, err := im.GetIntent(id); err == nil {
		t.Error("rejected intent should be removed from the queue")
	}

	// Feedback callback fires (async via goroutine).
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("feedback callback did not fire")
	}
	mu.Lock()
	if gotReason != "Rejected by user" {
		t.Errorf("feedback reason = %q, want %q", gotReason, "Rejected by user")
	}
	mu.Unlock()

	// Rejecting a missing intent errors.
	if err := im.RejectIntent("missing", "x"); err == nil {
		t.Error("RejectIntent for missing id should error")
	}
}

// TestIntentManager_RejectAfterClaim covers the cleanup path used by the
// authorize handler when, after claiming an intent for execution, the
// stale-price guard or the broker call fails. Before the fix, RejectIntent
// only accepted PENDING intents, so these intents leaked as zombie EXECUTING
// entries (the sweeper only expires PENDING) and the agent got no feedback.
func TestIntentManager_RejectAfterClaim(t *testing.T) {
	im := newTestIntentManager(300)
	id, _ := im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "sell", 1, 5000)

	if _, err := im.ClaimForExecution(id); err != nil {
		t.Fatalf("ClaimForExecution: %v", err)
	}

	if err := im.RejectIntent(id, "execution failed"); err != nil {
		t.Fatalf("RejectIntent after claim should succeed, got: %v", err)
	}
	if _, err := im.GetIntent(id); err == nil {
		t.Error("intent should be removed after reject-from-executing")
	}
}

func TestIntentManager_MarkCompletedRemoves(t *testing.T) {
	im := newTestIntentManager(300)
	id, _ := im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "sell", 1, 5000)
	im.ClaimForExecution(id)
	im.MarkCompleted(id)
	if _, err := im.GetIntent(id); err == nil {
		t.Error("completed intent should be removed from the queue")
	}
}

func TestIntentManager_TTLSweepExpiresPendingOnly(t *testing.T) {
	// TTL of 0 seconds → any pending intent is immediately stale.
	im := newTestIntentManager(0)

	var expired int64
	im.SetFeedbackCallback(func(intent *Intent, reason string) {
		atomic.AddInt64(&expired, 1)
	})

	pendingID, _ := im.CreateIntent(IntentTypeManagedPosition, []byte(`{}`), "ESTX50", "buy", 1, 5000)

	// An EXECUTING intent must NOT be swept even though it is past TTL —
	// otherwise an in-flight authorize could be yanked mid-execution.
	execID, _ := im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "sell", 1, 5000)
	im.ClaimForExecution(execID)

	time.Sleep(5 * time.Millisecond) // ensure CreatedAt is in the past vs TTL 0
	im.sweep()

	if _, err := im.GetIntent(pendingID); err == nil {
		t.Error("pending intent past TTL should be swept")
	}
	if _, err := im.GetIntent(execID); err != nil {
		t.Error("executing intent must NOT be swept by TTL")
	}

	// Wait for the async expiry feedback for the pending intent.
	deadline := time.After(time.Second)
	for atomic.LoadInt64(&expired) < 1 {
		select {
		case <-deadline:
			t.Fatal("expiry feedback callback did not fire")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestIntentManager_ConcurrentCreateAndList(t *testing.T) {
	// Run with -race to catch map races.
	im := newTestIntentManager(300)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); im.CreateIntent(IntentTypeOptionsOrder, []byte(`{}`), "ESTX50", "buy", 1, 5000) }()
		go func() { defer wg.Done(); _ = im.ListIntents() }()
	}
	wg.Wait()
	if n := len(im.ListIntents()); n != 100 {
		t.Errorf("expected 100 intents, got %d", n)
	}
}
