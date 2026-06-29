package cache

import (
	"context"
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

// Allow implements a sliding-window rate limiter backed by a Redis sorted set.
func (r *RedisCache) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-window).UnixNano()
	member := fmt.Sprintf("%d", now.UnixNano())

	pipe := r.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixNano()), Member: member})
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, window+time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return false, fmt.Errorf("cache: redis pipeline: %w", err)
	}
	return countCmd.Val() <= limit, nil
}

// Ensure RedisCache implements Cache at compile time.
var _ Cache = (*RedisCache)(nil)
