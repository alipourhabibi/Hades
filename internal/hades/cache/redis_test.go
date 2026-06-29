package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/redis/go-redis/v9"
)

func newTestRedisCache(t *testing.T) (*cache.RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return cache.NewRedisCache(client), mr
}

func TestRedisCache_GetSet(t *testing.T) {
	c, mr := newTestRedisCache(t)
	defer mr.Close()
	ctx := context.Background()

	if err := c.Set(ctx, "k", "v", 0); err != nil {
		t.Fatal(err)
	}
	val, ok := c.Get(ctx, "k")
	if !ok || val != "v" {
		t.Fatalf("Get: got (%q, %v), want (\"v\", true)", val, ok)
	}
}

func TestRedisCache_Incr(t *testing.T) {
	c, mr := newTestRedisCache(t)
	defer mr.Close()
	ctx := context.Background()

	v1, _ := c.Incr(ctx, "counter")
	v2, _ := c.Incr(ctx, "counter")
	if v1 != 1 || v2 != 2 {
		t.Fatalf("Incr: got %d, %d; want 1, 2", v1, v2)
	}
}

func TestRedisCache_Allow_SlidingWindow(t *testing.T) {
	c, mr := newTestRedisCache(t)
	defer mr.Close()
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

	allowed, err := c.Allow(ctx, "rl-key", limit, window)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("call 4 should be denied (over limit)")
	}
}
