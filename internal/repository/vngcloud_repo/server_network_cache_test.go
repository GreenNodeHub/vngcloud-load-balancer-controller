package vngcloud_repo

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
)

func testInfo(id string) serverNetworkInfo {
	return serverNetworkInfo{
		zoneID:     common.Zone("HAN01-1A"),
		networkID:  "net-" + id,
		subnetID:   "sub-" + id,
		subnetCIDR: "10.250.2.0/24",
	}
}

func TestServerNetworkCacheHitAndMiss(t *testing.T) {
	c := newServerNetworkCache(time.Hour, 16)

	_, ok := c.get("ins-1")
	assert.False(t, ok, "empty cache must miss")

	c.put("ins-1", testInfo("1"))
	got, ok := c.get("ins-1")
	assert.True(t, ok)
	assert.Equal(t, testInfo("1"), got)

	_, ok = c.get("ins-2")
	assert.False(t, ok, "a different server must not hit")
}

// The whole point of the TTL is that a wrong answer cannot outlive it, so expiry has to
// be a real miss rather than a stale hit.
func TestServerNetworkCacheExpires(t *testing.T) {
	now := time.Now()
	c := newServerNetworkCache(10*time.Minute, 16)
	c.now = func() time.Time { return now }

	c.put("ins-1", testInfo("1"))
	_, ok := c.get("ins-1")
	assert.True(t, ok, "fresh entry hits")

	now = now.Add(9 * time.Minute)
	_, ok = c.get("ins-1")
	assert.True(t, ok, "still inside the TTL")

	now = now.Add(2 * time.Minute)
	_, ok = c.get("ins-1")
	assert.False(t, ok, "past the TTL it must miss, not serve a stale answer")
}

// A cache that grows without a bound is a memory leak wearing a hat.
func TestServerNetworkCacheStaysBounded(t *testing.T) {
	const max = 8
	c := newServerNetworkCache(time.Hour, max)

	for i := 0; i < max*20; i++ {
		c.put(fmt.Sprintf("ins-%d", i), testInfo("x"))
		assert.LessOrEqual(t, c.len(), max, "cache must never exceed maxSize")
	}
}

// Every reconcile can look up the same nodes concurrently; run with -race.
func TestServerNetworkCacheConcurrent(t *testing.T) {
	c := newServerNetworkCache(time.Hour, 64)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("ins-%d", i%5)
			c.put(id, testInfo(id))
			c.get(id)
			c.len()
		}(i)
	}
	wg.Wait()
}

// GetServerNetworkInfo must answer from the cache without touching the SDK. The
// repository here has a nil client on purpose: if the read-through path were not taken,
// the call would panic instead of returning.
func TestGetServerNetworkInfoServesFromCacheWithoutCallingSDK(t *testing.T) {
	repo := &vngCloudRepository{
		serverNetworkCache: newServerNetworkCache(time.Hour, 16),
	}
	want := testInfo("cached")
	repo.serverNetworkCache.put("ins-cached", want)

	zone, networkID, subnetID, cidr, err := repo.GetServerNetworkInfo(context.Background(), "ins-cached")

	assert.NoError(t, err)
	assert.Equal(t, want.zoneID, zone)
	assert.Equal(t, want.networkID, networkID)
	assert.Equal(t, want.subnetID, subnetID)
	assert.Equal(t, want.subnetCIDR, cidr)
}
