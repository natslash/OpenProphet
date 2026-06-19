package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestBeat() *AutonomousBeat {
	lg := logrus.New()
	lg.SetLevel(logrus.PanicLevel) // quiet
	return NewAutonomousBeat(nil, nil, lg, AutonomousBeatConfig{Symbol: "TEST", Interval: time.Minute, MaxDailyExecutions: 3})
}

func TestAutonomousBeat_ExpiryAndHygiene(t *testing.T) {
	b := newTestBeat()
	now := time.Now()
	b.intents["live"] = &TradeIntent{ID: "live", ExpiresAt: now.Add(time.Minute)}
	b.intents["dead"] = &TradeIntent{ID: "dead", ExpiresAt: now.Add(-time.Minute)}

	// ListIntents hides the expired one.
	if got := b.ListIntents(); len(got) != 1 || got[0].ID != "live" {
		t.Fatalf("ListIntents = %+v, want only 'live'", got)
	}
	// expireIntents removes it from the store.
	b.expireIntents()
	if _, ok := b.intents["dead"]; ok {
		t.Error("expired intent should be removed from the store")
	}
	if _, ok := b.intents["live"]; !ok {
		t.Error("live intent should remain")
	}
}

func TestAutonomousBeat_DailyCapBlocksAuthorize(t *testing.T) {
	b := newTestBeat()
	b.executedDay = time.Now().Format("2006-01-02")
	b.executed = b.cfg.MaxDailyExecutions // at cap
	b.intents["x"] = &TradeIntent{ID: "x", ExpiresAt: time.Now().Add(time.Minute)}

	// Cap is checked before any placement, so a nil PositionManager is fine.
	if _, err := b.Authorize(context.Background(), "x"); err == nil {
		t.Fatal("Authorize should be blocked at the daily cap")
	}
}

func TestAutonomousBeat_AuthorizeUnknownAndExpired(t *testing.T) {
	b := newTestBeat()
	if _, err := b.Authorize(context.Background(), "missing"); err == nil {
		t.Error("unknown intent should error")
	}
	b.intents["old"] = &TradeIntent{ID: "old", ExpiresAt: time.Now().Add(-time.Second)}
	if _, err := b.Authorize(context.Background(), "old"); err == nil {
		t.Error("expired intent should error")
	}
	if _, ok := b.intents["old"]; ok {
		t.Error("expired intent should be evicted on authorize attempt")
	}
}

func TestAutonomousBeat_ConcurrentAccess(t *testing.T) {
	b := newTestBeat()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(n int) { defer wg.Done(); b.mu.Lock(); b.intents[string(rune(n))] = &TradeIntent{ExpiresAt: time.Now().Add(time.Minute)}; b.mu.Unlock() }(i)
		go func() { defer wg.Done(); _ = b.ListIntents() }()
		go func() { defer wg.Done(); b.expireIntents() }()
	}
	wg.Wait()
}
