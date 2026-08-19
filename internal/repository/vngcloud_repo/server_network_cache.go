package vngcloud_repo

import (
	"sync"
	"time"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
)

const (
	// A server's zone, network, subnet and that subnet's CIDR are properties of its
	// primary interface: they do not change while the server exists. An hour is well
	// inside that lifetime while still bounding how long a wrong answer could survive
	// if one ever were cached.
	serverNetworkCacheTTL = time.Hour

	// Generous next to any real cluster's node count, so eviction is a safety net
	// rather than something the cache does during normal operation.
	serverNetworkCacheMaxSize = 4096
)

// serverNetworkInfo is what GetServerNetworkInfo resolves for a single server.
type serverNetworkInfo struct {
	zoneID     common.Zone
	networkID  string
	subnetID   string
	subnetCIDR string
}

type serverNetworkCacheEntry struct {
	info      serverNetworkInfo
	expiresAt time.Time
}

// serverNetworkCache remembers where a server sits on the network.
//
// The controller resolves this once per node per Ingress on every resync, which makes it
// the largest single source of vserver API calls - and those calls are what exhausts the
// per-project rate limit. The answers are immutable for the server's lifetime, so they
// are worth remembering.
//
// Deliberately small: entries expire, the map is bounded, and exactly one instance is
// built per process in NewVngCloudRepository. A cache allocated per reconcile has cost
// us an OOM before.
type serverNetworkCache struct {
	mu      sync.RWMutex
	entries map[string]serverNetworkCacheEntry
	ttl     time.Duration
	maxSize int

	// now is a field so tests can advance the clock instead of sleeping.
	now func() time.Time
}

func newServerNetworkCache(ttl time.Duration, maxSize int) *serverNetworkCache {
	return &serverNetworkCache{
		entries: make(map[string]serverNetworkCacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
		now:     time.Now,
	}
}

// get returns the remembered info for serverID, or false when it is absent or expired.
func (c *serverNetworkCache) get(serverID string) (serverNetworkInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[serverID]
	if !ok || c.now().After(entry.expiresAt) {
		return serverNetworkInfo{}, false
	}
	return entry.info, true
}

// put remembers info for serverID. Only successful lookups should be stored: caching a
// NotFound would keep a node that is still being created unreachable for the whole TTL.
func (c *serverNetworkCache) put(serverID string, info serverNetworkInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictLocked(serverID)
	}
	c.entries[serverID] = serverNetworkCacheEntry{
		info:      info,
		expiresAt: c.now().Add(c.ttl),
	}
}

// evictLocked makes room for one more entry. Expired entries go first; if none are
// expired it drops an arbitrary one, which is enough to keep the map bounded without
// carrying LRU bookkeeping for a cache this size.
func (c *serverNetworkCache) evictLocked(incoming string) {
	now := c.now()
	for id, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, id)
		}
	}
	for len(c.entries) >= c.maxSize {
		for id := range c.entries {
			if id == incoming {
				continue
			}
			delete(c.entries, id)
			break
		}
	}
}

func (c *serverNetworkCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
