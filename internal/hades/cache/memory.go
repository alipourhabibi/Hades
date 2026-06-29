package cache

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type memEntry struct {
	value   string
	expires int64 // Unix ns; 0 = no expiry
}

func (e *memEntry) expired() bool {
	return e.expires != 0 && time.Now().UnixNano() > e.expires
}

// windowEntry is a single sliding-window hit, stored as nanosecond timestamp.
type windowEntry struct {
	ts int64
}

// MemoryCache is an in-process, zero-dependency implementation of Cache.
// It is suitable for single-node self-host deployments and local development.
// Rate-limit state is not durable across restarts.
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*memEntry

	counterMu sync.Mutex
	counters  map[string]*atomic.Int64

	windowMu sync.Mutex
	windows  map[string][]windowEntry
}

// NewMemoryCache creates an in-process Cache with no external dependencies.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		entries:  make(map[string]*memEntry),
		counters: make(map[string]*atomic.Int64),
		windows:  make(map[string][]windowEntry),
	}
}

func (m *MemoryCache) Get(_ context.Context, key string) (string, bool) {
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || e.expired() {
		return "", false
	}
	return e.value, true
}

func (m *MemoryCache) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	var expires int64
	if ttl > 0 {
		expires = time.Now().Add(ttl).UnixNano()
	}
	m.mu.Lock()
	m.entries[key] = &memEntry{value: value, expires: expires}
	m.mu.Unlock()
	if ttl > 0 {
		time.AfterFunc(ttl, func() {
			m.mu.Lock()
			if e, ok := m.entries[key]; ok && e.expired() {
				delete(m.entries, key)
			}
			m.mu.Unlock()
		})
	}
	return nil
}

func (m *MemoryCache) Incr(_ context.Context, key string) (int64, error) {
	m.counterMu.Lock()
	c, ok := m.counters[key]
	if !ok {
		c = &atomic.Int64{}
		m.counters[key] = c
	}
	m.counterMu.Unlock()
	return c.Add(1), nil
}

func (m *MemoryCache) Allow(_ context.Context, key string, limit int64, window time.Duration) (bool, error) {
	now := time.Now().UnixNano()
	cutoff := now - window.Nanoseconds()

	m.windowMu.Lock()
	defer m.windowMu.Unlock()

	entries := m.windows[key]

	// Evict entries outside the window.
	start := sort.Search(len(entries), func(i int) bool { return entries[i].ts >= cutoff })
	entries = entries[start:]

	// Append current request.
	entries = append(entries, windowEntry{ts: now})
	m.windows[key] = entries

	if int64(len(entries)) > limit {
		return false, nil
	}
	return true, nil
}

// Ensure MemoryCache implements Cache at compile time.
var _ Cache = (*MemoryCache)(nil)
