package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache is a Redis-backed implementation of Cache.
// It uses a sorted-set sliding window for Allow.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache returns a Cache backed by the given Redis client.
func NewRedisCache(client *redis.Client) *RedisCache {
	return &RedisCache{client: client}
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, bool) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// slidingWindowScript trims, records, counts and expires a sliding window in
// one atomic server-side step.
//
// A pipeline is not enough: the four commands interleave with concurrent
// requests, so the observed count can be stale and the effective limit is
// exceeded under load. A Lua script runs to completion without interleaving.
//
// KEYS[1] = window key
// ARGV[1] = window start (nanoseconds, exclusive lower bound)
// ARGV[2] = now (nanoseconds, the score)
// ARGV[3] = unique member id
// ARGV[4] = key TTL in milliseconds
// Returns the number of hits in the window including this one.
var slidingWindowScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '0', ARGV[1])
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return redis.call('ZCARD', KEYS[1])
`)

// Allow implements a sliding-window rate limiter backed by a Redis sorted set.
//
// A Redis failure returns an error and reports the request as not allowed.
// Callers decide what to do with that: the auth handlers currently treat an
// error as "limiter unavailable, let the request through", which is a
// deliberate availability choice recorded at those call sites.
func (r *RedisCache) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-window).UnixNano()

	// The member must be unique per hit. Using the raw timestamp collides when
	// two requests land in the same nanosecond, and a sorted set silently
	// deduplicates the second one, undercounting the window.
	var randSuffix [8]byte
	if _, err := rand.Read(randSuffix[:]); err != nil {
		return false, fmt.Errorf("cache: redis rate limit: %w", err)
	}
	member := fmt.Sprintf("%d-%s", now.UnixNano(), hex.EncodeToString(randSuffix[:]))

	count, err := slidingWindowScript.Run(ctx, r.client,
		[]string{key},
		windowStart,
		now.UnixNano(),
		member,
		(window + time.Second).Milliseconds(),
	).Int64()
	if err != nil {
		return false, fmt.Errorf("cache: redis rate limit: %w", err)
	}
	return count <= limit, nil
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Ensure RedisCache implements Cache at compile time.
var _ Cache = (*RedisCache)(nil)
