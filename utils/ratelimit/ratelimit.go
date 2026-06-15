// Package ratelimit provides a Redis-backed sliding-window rate limiter.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter is a Redis-backed sliding-window rate limiter.
type Limiter struct {
	client *redis.Client
}

// New returns a Limiter backed by the given Redis client.
func New(client *redis.Client) *Limiter {
	return &Limiter{client: client}
}

// Allow checks whether key is within the limit for the given window.
// It returns true if the request is allowed, false if the limit is exceeded.
// The sliding window is implemented with a sorted set where each member is a
// unique timestamp-based entry and the score is the Unix timestamp in nanoseconds.
func (l *Limiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-window).UnixNano()
	member := fmt.Sprintf("%d", now.UnixNano())

	pipe := l.client.Pipeline()
	// Remove entries outside the window.
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	// Add the current request.
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixNano()), Member: member})
	// Count entries in the window.
	countCmd := pipe.ZCard(ctx, key)
	// Set expiry.
	pipe.Expire(ctx, key, window+time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("ratelimit: pipeline: %w", err)
	}
	return countCmd.Val() <= limit, nil
}
