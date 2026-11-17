package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// freshKey returns a rate-limit key for this test with no count left from an
// earlier run.
func freshKey(t *testing.T, suffix string) string {
	t.Helper()
	key := RateLimitKey("test", t.Name()+suffix)
	if err := database.Redis.Del(context.Background(), key).Err(); err != nil {
		t.Fatalf("clearing %s: %v", key, err)
	}
	return key
}

func TestHitAllowsUpToMaxThenRefuses(t *testing.T) {
	testutil.UseTestRedis(t)
	ctx := context.Background()
	l := Limit{Max: 3, Window: time.Minute}
	key := freshKey(t, "")

	for i := 1; i <= 3; i++ {
		if allowed, _ := Hit(ctx, key, l); !allowed {
			t.Fatalf("hit %d refused, want allowed", i)
		}
	}
	allowed, retryAfter := Hit(ctx, key, l)
	if allowed {
		t.Fatal("hit 4 allowed, want refused")
	}
	if retryAfter <= 0 || retryAfter > time.Minute {
		t.Fatalf("retryAfter = %v, want within (0, 1m]", retryAfter)
	}

	if allowed, _ := Hit(ctx, freshKey(t, "-other"), l); !allowed {
		t.Fatal("another key was refused; keys must count separately")
	}
}

func TestHitWindowEnds(t *testing.T) {
	testutil.UseTestRedis(t)
	ctx := context.Background()
	l := Limit{Max: 1, Window: time.Second}
	key := freshKey(t, "")

	Hit(ctx, key, l)
	if allowed, _ := Hit(ctx, key, l); allowed {
		t.Fatal("second hit allowed inside the window")
	}
	time.Sleep(1100 * time.Millisecond)
	if allowed, _ := Hit(ctx, key, l); !allowed {
		t.Fatal("hit refused after the window ended")
	}
}

// TestHitCountsConcurrentRequests guards the review finding on #40: requests
// that arrive together must not all pass before any is counted.
func TestHitCountsConcurrentRequests(t *testing.T) {
	testutil.UseTestRedis(t)
	ctx := context.Background()
	l := Limit{Max: 5, Window: time.Minute}
	key := freshKey(t, "")

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := Hit(ctx, key, l); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 5 {
		t.Fatalf("%d of 50 concurrent hits allowed, want 5", got)
	}
}

func TestClearRateLimitsResetsTheCount(t *testing.T) {
	testutil.UseTestRedis(t)
	ctx := context.Background()
	l := Limit{Max: 1, Window: time.Minute}
	key := freshKey(t, "")

	Hit(ctx, key, l)
	ClearRateLimits(ctx, key)
	if ok, _ := Hit(ctx, key, l); !ok {
		t.Fatal("hit refused after ClearRateLimits")
	}
}

func TestRateLimitAllowsWhenRedisIsDown(t *testing.T) {
	testutil.UseBrokenRedis(t)
	ctx := context.Background()
	l := Limit{Max: 1, Window: time.Minute}
	key := RateLimitKey("test", t.Name())

	for range 3 {
		if allowed, _ := Hit(ctx, key, l); !allowed {
			t.Fatal("Hit refused while Redis is down; want allowed")
		}
	}
	ClearRateLimits(ctx, key) // only logs
}
