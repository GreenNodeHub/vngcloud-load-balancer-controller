package vngcloud_repo

import (
	"context"
	"testing"
	"time"

	entityv2 "github.com/GreenNodeHub/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/stretchr/testify/assert"
)

// GetSubnetByID must answer from the cache without touching the SDK. The repository here has a
// nil client on purpose: if the read-through path were not taken, the call would panic instead
// of returning.
func TestGetSubnetByIDServesFromCacheWithoutCallingSDK(t *testing.T) {
	repo := &vngCloudRepository{
		subnetCache: newTTLCache[*entityv2.Subnet](time.Hour, 16),
	}
	want := &entityv2.Subnet{Id: "sub-1", NetworkId: "net-1", Cidr: "10.250.2.0/24", ZoneID: "HAN01-1A"}
	repo.subnetCache.put(subnetCacheKey("net-1", "sub-1"), want)

	got, err := repo.GetSubnetByID(context.Background(), "net-1", "sub-1")

	assert.NoError(t, err)
	assert.Equal(t, want.Id, got.Id)
	assert.Equal(t, want.Cidr, got.Cidr)
	assert.Equal(t, want.ZoneID, got.ZoneID)
}

// The key has to carry both ids. Two networks can hold subnets with ids that collide, and
// answering for the wrong network would hand back a CIDR from somewhere else entirely.
func TestSubnetCacheKeySeparatesNetworks(t *testing.T) {
	assert.NotEqual(t, subnetCacheKey("net-1", "sub-1"), subnetCacheKey("net-2", "sub-1"))
}
