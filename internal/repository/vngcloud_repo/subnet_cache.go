package vngcloud_repo

import (
	"time"
)

const (
	// A subnet's zone and CIDR are fixed for its lifetime, which is what the callers read:
	// build_lbc resolves a backend/prefer subnet down to (ZoneID, Id, Cidr) and nothing else.
	// An hour is well inside that lifetime while still bounding how long a wrong answer could
	// survive if one ever were cached.
	subnetCacheTTL = time.Hour

	// Generous next to the number of subnets one cluster's load balancers can sit on, so
	// eviction is a safety net rather than something the cache does during normal operation.
	subnetCacheMaxSize = 512
)

// subnetCacheKey carries both ids. Subnet ids are only unique within a network, so keying on
// the subnet alone would let one network's answer be served for another - handing back a CIDR
// from somewhere else entirely.
func subnetCacheKey(networkID, subnetID string) string {
	return networkID + "/" + subnetID
}
