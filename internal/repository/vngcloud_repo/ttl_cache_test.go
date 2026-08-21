package vngcloud_repo

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTTLCacheHitAndMiss(t *testing.T) {
	c := newTTLCache[string](time.Hour, 16)

	_, ok := c.get("k1")
	assert.False(t, ok, "empty cache must miss")

	c.put("k1", "v1")
	got, ok := c.get("k1")
	assert.True(t, ok)
	assert.Equal(t, "v1", got)

	_, ok = c.get("k2")
	assert.False(t, ok, "a different key must not hit")
}

// The whole point of the TTL is that a wrong answer cannot outlive it, so expiry has to
// be a real miss rather than a stale hit.
func TestTTLCacheExpires(t *testing.T) {
	now := time.Now()
	c := newTTLCache[string](10*time.Minute, 16)
	c.now = func() time.Time { return now }

	c.put("k1", "v1")
	_, ok := c.get("k1")
	assert.True(t, ok, "fresh entry hits")

	now = now.Add(9 * time.Minute)
	_, ok = c.get("k1")
	assert.True(t, ok, "still inside the TTL")

	now = now.Add(2 * time.Minute)
	_, ok = c.get("k1")
	assert.False(t, ok, "past the TTL it must miss, not serve a stale answer")
}

// invalidate is how a caller that is about to write forces itself a fresh read, so it has
// to be an immediate miss and it must not disturb the other keys.
func TestTTLCacheInvalidate(t *testing.T) {
	c := newTTLCache[string](time.Hour, 16)
	c.put("k1", "v1")
	c.put("k2", "v2")

	c.invalidate("k1")

	_, ok := c.get("k1")
	assert.False(t, ok, "invalidated key must miss straight away")
	got, ok := c.get("k2")
	assert.True(t, ok, "invalidate must not touch other keys")
	assert.Equal(t, "v2", got)

	c.invalidate("absent") // must not panic
}

// A cache that grows without a bound is a memory leak wearing a hat.
func TestTTLCacheStaysBounded(t *testing.T) {
	const max = 8
	c := newTTLCache[string](time.Hour, max)

	for i := 0; i < max*20; i++ {
		c.put(fmt.Sprintf("k%d", i), "v")
		assert.LessOrEqual(t, c.len(), max, "cache must never exceed maxSize")
	}
}

// Every reconcile can look up the same keys concurrently; run with -race.
func TestTTLCacheConcurrent(t *testing.T) {
	c := newTTLCache[string](time.Hour, 64)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%5)
			c.put(key, "v")
			c.get(key)
			c.invalidate(fmt.Sprintf("k%d", (i+1)%5))
			c.len()
		}(i)
	}
	wg.Wait()
}
