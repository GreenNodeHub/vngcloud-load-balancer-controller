package lbc_uc

import (
	"context"

	"github.com/pkg/errors"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
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
	return t.deleteRedundantTagsFrom(ctx, lbId, t.lbConfig.Status.CreatedTags)
}

// deleteRedundantTagsFrom is deleteRedundantTags with the created-tag record made explicit,
// so a migration can release the retiring load balancer's tags from its snapshot after the
// live status has moved on to the new load balancer.
func (t *defaultModelDeployTask) deleteRedundantTagsFrom(ctx context.Context, lbId string, recordedTags map[string]string) error {
	// The cluster id belongs to the cluster, not to this LBC: sibling LBCs may still point at
	// the same load balancer, and this path is reached precisely when the load balancer still
	// has resources on it - possibly another LBC's. So the id may only come off once nothing
	// else in the cluster references it.
	//
	// Looked up lazily, and only when there is a cluster tag to take off, because it costs a
	// list of every LBC in the cluster. ensureTags runs the closure twice, hence the memo.
	var (
		lookedUp  bool
		lookupErr error
		shared    bool
	)
	stillShared := func() bool {
		if !lookedUp {
			lookedUp = true
			shared, lookupErr = t.loadBalancerHasOtherLBC(ctx, lbId)
			if lookupErr != nil {
				// Keep the tag while we cannot tell. The error is returned below, so this
				// reconcile is retried rather than trusted.
				return true
			}
		}
		return shared
	}

	err := t.ensureTags(ctx, lbId, func(currentTags map[string]string) (map[string]string, map[string]string) {
		ensuredTags := make(map[string]string)

		createdTags := make(map[string]string, len(recordedTags))
		for k, v := range recordedTags {
			createdTags[k] = v
		}

		// ensure ClusterTagKey tag and DeprecatedClusterTagKey tag
		if _, hasClusterTag := currentTags[domain.ClusterTagKey]; hasClusterTag && stillShared() {
			// Not ours to drop while a sibling still uses the load balancer. Taking it out of
			// the created set is what keeps it out of the removal the write would otherwise
			// perform.
			delete(createdTags, domain.ClusterTagKey)
		} else if t.lbConfig.Spec.ClusterId != nil {
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

		// The provenance tag has to outlive this LBC - that is its whole purpose - so it is
		// not ours to drop on the way out either.
		delete(createdTags, domain.CreatedByClusterTagKey)

		return ensuredTags, createdTags
	})
	if lookupErr != nil {
		return lookupErr
	}
	return err
}

// loadBalancerHasOtherLBC reports whether a LoadBalancerConfig other than this one still
// points at the load balancer. LBCs sharing a load balancer can live in different
// namespaces, so the whole cluster is searched - the same way validateCrossLBCs does it.
//
// An LBC already being deleted does not count: it is on its way out, and whichever of the
// two finishes last is the one that should take the cluster id with it.
func (t *defaultModelDeployTask) loadBalancerHasOtherLBC(ctx context.Context, lbId string) (bool, error) {
	allLBCs := &v1alpha1.LoadBalancerConfigList{}
	if err := t.k8sRepo.ListLoadBalancerConfig(ctx, allLBCs); err != nil {
		return false, errors.Wrap(err, "failed to list LoadBalancerConfigs to see who else uses the load balancer")
	}

	for i := range allLBCs.Items {
		other := &allLBCs.Items[i]
		if other.UID == t.lbConfig.UID || other.DeletionTimestamp != nil {
			continue
		}
		// Status is where a deployed load balancer is recorded; Spec covers one that is
		// pinned but has not been deployed yet.
		for _, id := range []*string{other.Status.LoadBalancerId, other.Spec.LoadBalancerId} {
			if id != nil && *id == lbId {
				t.logger.Infof("Load balancer %s is still used by LBC %s/%s, keeping the cluster tag",
					lbId, other.Namespace, other.Name)
				return true, nil
			}
		}
	}
	return false, nil
}
