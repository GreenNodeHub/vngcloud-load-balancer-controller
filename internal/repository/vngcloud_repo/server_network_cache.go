package vngcloud_repo

import (
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
//
// The controller resolves this once per node per Ingress on every resync, which makes it
// the largest single source of vserver API calls. The answers are immutable for the
// server's lifetime, so they are worth remembering; see ttlCache.
type serverNetworkInfo struct {
	zoneID     common.Zone
	networkID  string
	subnetID   string
	subnetCIDR string
}
