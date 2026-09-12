package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/cache"
)

func TestMemoryCache_GetSet(t *testing.T) {
	c := cache.NewMemoryCache()
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", 0); err != nil {
		t.Fatal(err)
	}
	val, ok, err := c.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || val != "v" {
		t.Fatalf("Get: got (%q, %v), want (\"v\", true)", val, ok)
	}
}

func TestMemoryCache_GetMissing(t *testing.T) {
	c := cache.NewMemoryCache()
	_, ok, err := c.Get(context.Background(), "no-such-key")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestMemoryCache_TTLExpiry(t *testing.T) {
	c := cache.NewMemoryCache()
	ctx := context.Background()

	if err := c.Set(ctx, "ttl-key", "x", 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	// Key should exist immediately.
	if _, ok, _ := c.Get(ctx, "ttl-key"); !ok {
		t.Fatal("expected key to exist before TTL expires")
	}
	time.Sleep(100 * time.Millisecond)
	// Key should be gone after TTL.
	if _, ok, _ := c.Get(ctx, "ttl-key"); ok {
		t.Fatal("expected key to be expired")
	}
}

func TestMemoryCache_Allow_SlidingWindow(t *testing.T) {
	c := cache.NewMemoryCache()
	ctx := context.Background()

	const limit = 3
	const window = 10 * time.Second

	for i := 0; i < limit; i++ {
		allowed, err := c.Allow(ctx, "rl-key", limit, window)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}

	// Next call should exceed limit.
	allowed, err := c.Allow(ctx, "rl-key", limit, window)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("call 4 should be denied (over limit)")
	}
}
