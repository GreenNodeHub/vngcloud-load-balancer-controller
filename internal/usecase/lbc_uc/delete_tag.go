package lbc_uc

import (
	"context"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// deleteRedundantTags removes the tags created by this cluster from the load balancer.
// It removes the cluster ID from cluster tags (both current and deprecated).
// If the tag value becomes empty after removal, the tag key is deleted.
// VpcTagKey and BillingTagKey are only deleted if no other cluster is using the LB.
// Also updates the CreatedTags in status.
//
// Like deployTags this goes through ensureTags, so the value it writes is always computed
// from a fresh read: removing this cluster's id from a stale list would resurrect an id
// another cluster had just removed, or drop one it had just added.
func (t *defaultModelDeployTask) deleteRedundantTags(ctx context.Context, lbId string) error {
	return t.ensureTags(ctx, lbId, func(currentTags map[string]string) (map[string]string, map[string]string) {
		ensuredTags := make(map[string]string)

		createdTags := make(map[string]string, len(t.lbConfig.Status.CreatedTags))
		for k, v := range t.lbConfig.Status.CreatedTags {
			createdTags[k] = v
		}

		// ensure ClusterTagKey tag and DeprecatedClusterTagKey tag
		if t.lbConfig.Spec.ClusterId != nil {
			// ensure remove ClusterTagKey
			vksClusterValue := currentTags[domain.ClusterTagKey]
			vksClusterValue = removeTagValue(vksClusterValue, *t.lbConfig.Spec.ClusterId, domain.ClusterTagValueSeparator)
			if vksClusterValue != "" {
				ensuredTags[domain.ClusterTagKey] = vksClusterValue
			}

			// // ensure remove DeprecatedClusterTagKey
			// deprecatedVksClusterValue := currentTags[domain.DeprecatedClusterTagKey]
			// deprecatedVksClusterValue = removeTagValue(deprecatedVksClusterValue, *t.lbConfig.Spec.ClusterId, domain.DeprecatedClusterTagValueSeparator)
			// if deprecatedVksClusterValue != "" {
			// 	ensuredTags[domain.DeprecatedClusterTagKey] = deprecatedVksClusterValue
			// }
		}

		// ignore delete VpcTagKey and BillingTagKey
		delete(createdTags, domain.VpcTagKey)
		delete(createdTags, domain.BillingTagKey)

		return ensuredTags, createdTags
	})
}
