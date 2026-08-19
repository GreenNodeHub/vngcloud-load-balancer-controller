package vngcloud_repo

import (
	"context"
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

// GetServerNetworkInfo must answer from the cache without touching the SDK. The
// repository here has a nil client on purpose: if the read-through path were not taken,
// the call would panic instead of returning.
func TestGetServerNetworkInfoServesFromCacheWithoutCallingSDK(t *testing.T) {
	repo := &vngCloudRepository{
		serverNetworkCache: newTTLCache[serverNetworkInfo](time.Hour, 16),
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
