package cache

import (
	"fmt"

	"github.com/alipourhabibi/Hades/config"
	"github.com/redis/go-redis/v9"
)

// New constructs the Cache implementation selected by cfg.Backends.Cache.
// Returns a MemoryCache when the backend is "memory" or unset.
// Returns a RedisCache when the backend is "redis"; redisCfg must be populated.
func New(cfg config.BackendsConfig, redisCfg config.RedisConfig) (Cache, error) {
	backend := cfg.Cache
	if backend == "" {
		backend = "memory"
	}

	switch backend {
	case "memory":
		return NewMemoryCache(), nil
	case "redis":
		if redisCfg.Addr == "" {
			return nil, fmt.Errorf("cache: redis backend selected but redis.addr is not configured")
		}
		client := redis.NewClient(&redis.Options{
			Addr:     redisCfg.Addr,
			Password: redisCfg.Password,
			DB:       redisCfg.DB,
		})
		return NewRedisCache(client), nil
	default:
		return nil, fmt.Errorf("cache: unknown backend %q (valid: memory, redis)", backend)
	}
}
