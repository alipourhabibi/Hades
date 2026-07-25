// Package cache defines the Cache interface for control-plane operations
// (rate limiting, throttling, quotas, key-value storage). Implementations
// include MemoryCache (default, zero-dep) and RedisCache (production).
package cache

import (
	"context"
	"time"
)

// Cache is the abstraction for all control-plane state operations.
type Cache interface {
	// Get retrieves the string value stored at key.
	// Returns ("", false) when the key does not exist or has expired.
	Get(ctx context.Context, key string) (string, bool)

	// Set stores value at key with the given TTL.
	// A zero TTL means the entry never expires.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Incr atomically increments the integer stored at key by 1 and returns
	// the new value. If the key does not exist it is initialised to 0 before
	// incrementing.
	Incr(ctx context.Context, key string) (int64, error)

	// Allow implements a sliding-window rate limiter for key.
	// Returns true if the current request is within the limit, false if it is
	// over the limit for the given window duration.
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)
}
