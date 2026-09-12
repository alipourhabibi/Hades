// Package cache defines the Cache interface for control-plane operations
// (rate limiting, throttling, quotas, key-value storage). Implementations
// include MemoryCache (default, zero-dep) and RedisCache (production).
package cache

import (
	"context"
	"time"
)

// Cache is the abstraction for all control-plane state operations.
//
// Every method reports backend failure as an error, and every implementation
// must report failure the same way. An interface that cannot distinguish "not
// present" from "the backend is down" forces each caller to guess, and the two
// implementations guessing differently is how a rate limiter silently changes
// its fail-open posture when the backend is switched.
type Cache interface {
	// Get retrieves the string value stored at key.
	//
	// Returns ("", false, nil) when the key does not exist or has expired, and
	// a non-nil error when the backend could not answer. A caller that treats
	// an error as a miss is making a decision and should say so at the call
	// site.
	Get(ctx context.Context, key string) (string, bool, error)

	// Set stores value at key with the given TTL.
	// A zero TTL means the entry never expires.
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Allow implements a sliding-window rate limiter for key.
	//
	// Returns true if the current request is within the limit and false if it
	// is over the limit for the given window. A non-nil error means the limiter
	// could not decide; the boolean is false in that case and must not be read
	// as a deliberate deny. See internal/hades/server/auth.enforceLimit for the
	// project-wide policy on what to do with it.
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error)

	// Delete removes the entry at key. No-ops if the key does not exist.
	Delete(ctx context.Context, key string) error
}
