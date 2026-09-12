package cache

import (
	"context"
	"sort"
	"sync"
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

// maxValueKeys caps the plain key-value map for the same reason.
//
// GetOAuthURL is unauthenticated and writes an "oauth:state:<value>" entry per
// call, so this map is reachable without credentials too. Only the window map
// was capped before, which left the larger of the two maps unbounded.
const maxValueKeys = 100_000

// evictionSampleSize is how many keys are examined when the cache is over its
// cap. Sampling and dropping the oldest of the sample approximates LRU at
// constant cost. Sorting the whole map instead, which is what this replaced,
// turned a key-spray attack into an O(n log n) serialised operation per
// request, under the lock, on the path that exists to survive that attack.
const evictionSampleSize = 32

// MemoryCache is an in-process, zero-dependency implementation of Cache.
// It is suitable for single-node self-host deployments and local development.
// Rate-limit state is not durable across restarts.
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*memEntry

	windowMu sync.Mutex
	windows  map[string][]windowEntry
	// windowTTL is how long a window with no new hits is retained before the
	// janitor removes it.
	windowTTL time.Duration

	stopOnce sync.Once
	stop     chan struct{}
}

// NewMemoryCache creates an in-process Cache with no external dependencies.
//
// It starts a background janitor that evicts expired entries and idle
// rate-limit windows. Without it, every key ever seen is retained for the
// lifetime of the process.
func NewMemoryCache() *MemoryCache {
	m := &MemoryCache{
		entries:   make(map[string]*memEntry),
		windows:   make(map[string][]windowEntry),
		windowTTL: time.Hour,
		stop:      make(chan struct{}),
	}
	go m.janitor()
	return m
}

// Close stops the background janitor. It is safe to call more than once.
func (m *MemoryCache) Close() error {
	m.stopOnce.Do(func() { close(m.stop) })
	return nil
}

// janitor periodically drops expired value entries and rate-limit windows whose
// most recent hit is older than windowTTL.
//
// It is the only thing that removes expired entries. The previous
// implementation also scheduled a time.AfterFunc per Set, which meant one live
// timer and closure per write on high-churn keys such as OAuth state, doing
// work the janitor already does.
func (m *MemoryCache) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.sweep()
		case <-m.stop:
			return
		}
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

// evictWindowsLocked drops sampled least-recently-used windows until the map is
// back under maxRateLimitKeys. The caller must hold windowMu.
func (m *MemoryCache) evictWindowsLocked() {
	for len(m.windows) > maxRateLimitKeys {
		var victim string
		var victimTS int64
		seen := 0
		for k, entries := range m.windows {
			var last int64
			if len(entries) > 0 {
				last = entries[len(entries)-1].ts
			}
			if victim == "" || last < victimTS {
				victim, victimTS = k, last
			}
			if seen++; seen >= evictionSampleSize {
				break
			}
		}
		if victim == "" {
			return
		}
		delete(m.windows, victim)
	}
}

// evictEntriesLocked drops sampled entries until the value map is back under
// maxValueKeys, preferring already-expired entries. The caller must hold mu.
func (m *MemoryCache) evictEntriesLocked() {
	for len(m.entries) > maxValueKeys {
		var victim string
		var victimExpiry int64
		seen := 0
		for k, e := range m.entries {
			if e.expired() {
				victim = k
				break
			}
			// A zero expiry never expires, so treat it as the furthest away.
			exp := e.expires
			if exp == 0 {
				exp = 1<<63 - 1
			}
			if victim == "" || exp < victimExpiry {
				victim, victimExpiry = k, exp
			}
			if seen++; seen >= evictionSampleSize {
				break
			}
		}
		if victim == "" {
			return
		}
		delete(m.entries, victim)
	}
}

func (m *MemoryCache) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.RLock()
	e, ok := m.entries[key]
	m.mu.RUnlock()
	if !ok || e.expired() {
		return "", false, nil
	}
	return e.value, true, nil
}

func (m *MemoryCache) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	var expires int64
	if ttl > 0 {
		expires = time.Now().Add(ttl).UnixNano()
	}
	m.mu.Lock()
	m.entries[key] = &memEntry{value: value, expires: expires}
	m.evictEntriesLocked()
	m.mu.Unlock()
	return nil
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
	m.evictWindowsLocked()

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
