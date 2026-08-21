package vngcloud_repo

import (
	"sync"
	"time"
)

// ttlCache is a small expiring, size-bounded map used to keep the controller from asking
// the VNGCloud API the same question on every reconcile.
//
// Those repeated reads are what exhausts the per-project rate limit: once the project's
// budget is gone every caller in it starts getting 429, which the SDK reports as
// "permission denied", and unrelated work across the whole project stalls. Each cache
// here removes one such read from the hot path.
//
// Deliberately minimal: entries expire, the map is bounded, and exactly one instance per
// kind is built per process in NewVngCloudRepository. A cache allocated per reconcile has
// cost us an OOM before.
//
// Callers must treat a returned value as read-only unless they copied it, since every
// hit hands out the same value.
type ttlCache[V any] struct {
	mu      sync.RWMutex
	entries map[string]ttlCacheEntry[V]
	ttl     time.Duration
	maxSize int

	// now is a field so tests can advance the clock instead of sleeping.
	now func() time.Time
}

type ttlCacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

func newTTLCache[V any](ttl time.Duration, maxSize int) *ttlCache[V] {
	return &ttlCache[V]{
		entries: make(map[string]ttlCacheEntry[V]),
		ttl:     ttl,
		maxSize: maxSize,
		now:     time.Now,
	}
}

// get returns the remembered value for key, or false when it is absent or expired.
func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || c.now().After(entry.expiresAt) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

// put remembers value for key. Only successful lookups should be stored: caching a
// NotFound would keep a resource that is still being created unusable for the whole TTL.
func (c *ttlCache[V]) put(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictLocked(key)
	}
	c.entries[key] = ttlCacheEntry[V]{
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	}
}

// invalidate forgets key, so the next get misses and the caller reads through.
func (c *ttlCache[V]) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// evictLocked makes room for one more entry. Expired entries go first; if none are
// expired it drops an arbitrary one, which is enough to keep the map bounded without
// carrying LRU bookkeeping for a cache this size.
func (c *ttlCache[V]) evictLocked(incoming string) {
	now := c.now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	for len(c.entries) >= c.maxSize {
		for key := range c.entries {
			if key == incoming {
				continue
			}
			delete(c.entries, key)
			break
		}
	}
}

func (c *ttlCache[V]) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
