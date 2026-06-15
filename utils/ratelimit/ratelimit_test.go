package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(t *testing.T) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	return New(client), mr
}

func TestAllow_UnderLimit(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		ok, err := limiter.Allow(ctx, "test:key", 5, time.Minute)
		require.NoError(t, err)
		assert.True(t, ok, "request %d should be allowed", i+1)
	}
}

func TestAllow_AtLimit(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()

	// Use up all 3 slots.
	for i := 0; i < 3; i++ {
		ok, err := limiter.Allow(ctx, "test:exact", 3, time.Minute)
		require.NoError(t, err)
		assert.True(t, ok)
	}

	// 4th request exceeds limit.
	ok, err := limiter.Allow(ctx, "test:exact", 3, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok, "4th request should be denied")
}

func TestAllow_WindowExpiry(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()

	const window = 20 * time.Millisecond

	// Fill the limit.
	for i := 0; i < 2; i++ {
		ok, err := limiter.Allow(ctx, "test:window", 2, window)
		require.NoError(t, err)
		assert.True(t, ok)
	}

	// Exceed limit.
	ok, err := limiter.Allow(ctx, "test:window", 2, window)
	require.NoError(t, err)
	assert.False(t, ok)

	// Wait for the window to expire (the sorted-set scores are real timestamps).
	time.Sleep(window + 5*time.Millisecond)

	// Should be allowed again.
	ok, err = limiter.Allow(ctx, "test:window", 2, window)
	require.NoError(t, err)
	assert.True(t, ok, "request should be allowed after window expires")
}

func TestAllow_IndependentKeys(t *testing.T) {
	limiter, _ := newTestLimiter(t)
	ctx := context.Background()

	// Fill key1.
	for i := 0; i < 2; i++ {
		_, _ = limiter.Allow(ctx, "key1", 2, time.Minute)
	}
	ok, err := limiter.Allow(ctx, "key1", 2, time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)

	// key2 should be unaffected.
	ok, err = limiter.Allow(ctx, "key2", 2, time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
}
