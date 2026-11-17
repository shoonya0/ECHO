package services

import (
	"context"
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

func TestExceededDoesNotCount(t *testing.T) {
	testutil.UseTestRedis(t)
	ctx := context.Background()
	l := Limit{Max: 2, Window: time.Minute}
	key := freshKey(t, "")

	for range 5 {
		if exceeded, _ := Exceeded(ctx, key, l); exceeded {
			t.Fatal("Exceeded reported true before any hit")
		}
	}
	Hit(ctx, key, l)
	Hit(ctx, key, l)
	if exceeded, _ := Exceeded(ctx, key, l); exceeded {
		t.Fatal("Exceeded true at Max hits; the limit allows Max")
	}
	Hit(ctx, key, l)
	exceeded, retryAfter := Exceeded(ctx, key, l)
	if !exceeded || retryAfter <= 0 {
		t.Fatalf("Exceeded = %v, %v after Max+1 hits; want true and a wait", exceeded, retryAfter)
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
	if exceeded, _ := Exceeded(ctx, key, l); exceeded {
		t.Fatal("Exceeded true while Redis is down; want false")
	}
}

func TestEmailRateLimitIDIgnoresCaseAndSpace(t *testing.T) {
	if a, b := EmailRateLimitID(" Ann@Example.com "), EmailRateLimitID("ann@example.com"); a != b {
		t.Fatalf("EmailRateLimitID gives %q and %q; want one bucket", a, b)
	}
}
