package freqcap_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorgomania/mini-ssp/internal/freqcap"
)

func newCapper(t *testing.T, limit int64, window time.Duration) (*freqcap.RedisCapper, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	rc, err := freqcap.NewRedisCapper(s.Addr(), limit, window)
	if err != nil {
		t.Fatalf("NewRedisCapper: %v", err)
	}
	return rc, s
}

func TestRedisCapper_NotCappedInitially(t *testing.T) {
	rc, _ := newCapper(t, 3, time.Hour)
	if rc.IsCapped(context.Background(), "u1", "adcorp") {
		t.Fatal("should not be capped on first check")
	}
}

func TestRedisCapper_Record_AllowsUpToLimit(t *testing.T) {
	rc, _ := newCapper(t, 3, time.Hour)
	ctx := context.Background()
	for i := range 3 {
		if !rc.Record(ctx, "u1", "adcorp") {
			t.Fatalf("record %d should be allowed", i+1)
		}
	}
}

func TestRedisCapper_Record_BlocksOverLimit(t *testing.T) {
	rc, _ := newCapper(t, 3, time.Hour)
	ctx := context.Background()
	for range 3 {
		rc.Record(ctx, "u1", "adcorp")
	}
	if rc.Record(ctx, "u1", "adcorp") {
		t.Fatal("4th record should be blocked")
	}
}

func TestRedisCapper_IsCapped_AfterLimit(t *testing.T) {
	rc, _ := newCapper(t, 2, time.Hour)
	ctx := context.Background()
	rc.Record(ctx, "u1", "adcorp")
	rc.Record(ctx, "u1", "adcorp")
	if !rc.IsCapped(ctx, "u1", "adcorp") {
		t.Fatal("should be capped after reaching limit")
	}
}

func TestRedisCapper_Window_Expiry(t *testing.T) {
	rc, s := newCapper(t, 2, time.Second)
	ctx := context.Background()
	rc.Record(ctx, "u1", "adcorp")
	rc.Record(ctx, "u1", "adcorp")

	s.FastForward(2 * time.Second)

	if rc.IsCapped(ctx, "u1", "adcorp") {
		t.Fatal("should not be capped after window expiry")
	}
	if !rc.Record(ctx, "u1", "adcorp") {
		t.Fatal("record should be allowed after window expiry")
	}
}

func TestRedisCapper_IsolatedPerUser(t *testing.T) {
	rc, _ := newCapper(t, 2, time.Hour)
	ctx := context.Background()
	rc.Record(ctx, "u1", "adcorp")
	rc.Record(ctx, "u1", "adcorp")

	if rc.IsCapped(ctx, "u2", "adcorp") {
		t.Fatal("u2 should not be affected by u1's cap")
	}
}

func TestRedisCapper_IsolatedPerAdvertiser(t *testing.T) {
	rc, _ := newCapper(t, 2, time.Hour)
	ctx := context.Background()
	rc.Record(ctx, "u1", "adcorp")
	rc.Record(ctx, "u1", "adcorp")

	if rc.IsCapped(ctx, "u1", "medianet") {
		t.Fatal("medianet cap should be independent of adcorp")
	}
}

func TestRedisCapper_SetRules_PerAdvertiser(t *testing.T) {
	rc, _ := newCapper(t, 10, time.Hour) // global limit=10
	rc.SetRules(map[string]freqcap.Rule{
		"quickads": {Limit: 1, Window: time.Hour},
	})
	ctx := context.Background()

	rc.Record(ctx, "u1", "quickads")
	if rc.Record(ctx, "u1", "quickads") {
		t.Fatal("quickads should be capped at 1 (per-advertiser rule)")
	}
	// adcorp uses global limit=10, should still be fine at 2
	rc.Record(ctx, "u1", "adcorp")
	if !rc.Record(ctx, "u1", "adcorp") {
		t.Fatal("adcorp should use global limit=10")
	}
}

func TestRedisCapper_Concurrent_DoesNotExceedLimit(t *testing.T) {
	const limit = 5
	rc, _ := newCapper(t, limit, time.Hour)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup
	allowed := make(chan struct{}, goroutines)

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if rc.Record(ctx, "u1", "adcorp") {
				allowed <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(allowed)

	if got := len(allowed); got != limit {
		t.Fatalf("expected exactly %d allowed records, got %d", limit, got)
	}
}
