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

// maxRateLimitKeys caps how many distinct rate-limit windows are tracked.
//
// Window keys embed the client IP ("login:ip:<ip>"), so without a cap a caller
// spraying distinct source addresses grows the map without bound until the
// process runs out of memory. When the cap is hit the oldest windows are
// dropped: losing rate-limit state for an idle key is acceptable, running out
// of memory is not.
const maxRateLimitKeys = 100_000

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
	// windowTTL is how long a window with no new hits is retained before the
	// janitor removes it.
	windowTTL time.Duration
}

// NewMemoryCache creates an in-process Cache with no external dependencies.
//
// It starts a background janitor that evicts expired entries and idle
// rate-limit windows. Without it, every key ever seen is retained for the
// lifetime of the process.
func NewMemoryCache() *MemoryCache {
	m := &MemoryCache{
		entries:   make(map[string]*memEntry),
		counters:  make(map[string]*atomic.Int64),
		windows:   make(map[string][]windowEntry),
		windowTTL: time.Hour,
	}
	go m.janitor()
	return m
}

// janitor periodically drops expired value entries and rate-limit windows whose
// most recent hit is older than windowTTL.
func (m *MemoryCache) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.sweep()
	}
}

func (m *MemoryCache) sweep() {
	m.mu.Lock()
	for k, e := range m.entries {
		if e.expired() {
			delete(m.entries, k)
		}
	}
	m.mu.Unlock()

	cutoff := time.Now().Add(-m.windowTTL).UnixNano()
	m.windowMu.Lock()
	for k, entries := range m.windows {
		if len(entries) == 0 || entries[len(entries)-1].ts < cutoff {
			delete(m.windows, k)
		}
	}
	m.windowMu.Unlock()
}

// evictOldestWindowsLocked drops the least recently used windows until the map
// is back under maxRateLimitKeys. The caller must hold windowMu.
func (m *MemoryCache) evictOldestWindowsLocked() {
	if len(m.windows) <= maxRateLimitKeys {
		return
	}
	target := len(m.windows) - maxRateLimitKeys
	type keyAge struct {
		key  string
		last int64
	}
	ages := make([]keyAge, 0, len(m.windows))
	for k, entries := range m.windows {
		var last int64
		if len(entries) > 0 {
			last = entries[len(entries)-1].ts
		}
		ages = append(ages, keyAge{key: k, last: last})
	}
	sort.Slice(ages, func(i, j int) bool { return ages[i].last < ages[j].last })
	for i := 0; i < target && i < len(ages); i++ {
		delete(m.windows, ages[i].key)
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
	m.evictOldestWindowsLocked()

	if int64(len(entries)) > limit {
		return false, nil
	}
	return true, nil
}

func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
	return nil
}

// Ensure MemoryCache implements Cache at compile time.
var _ Cache = (*MemoryCache)(nil)
