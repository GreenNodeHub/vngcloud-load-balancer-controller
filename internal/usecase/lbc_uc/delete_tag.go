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
func (t *defaultModelDeployTask) deleteRedundantTags(ctx context.Context, lbId string) error {
	currentTags := make(map[string]string)
	listTags, err := t.vngcloudRepo.ListTags(ctx, lbId)
	if err != nil {
		return err
	}
	for _, tag := range listTags.Items {
		// ignore system tag, not allow to modify
		if tag.SystemTag {
			continue
		}
		currentTags[tag.Key] = tag.Value
	}

	ensuredTags := make(map[string]string)

	createdTags := make(map[string]string)
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

		// ensure remove DeprecatedClusterTagKey
		deprecatedVksClusterValue := currentTags[domain.DeprecatedClusterTagKey]
		deprecatedVksClusterValue = removeTagValue(deprecatedVksClusterValue, *t.lbConfig.Spec.ClusterId, domain.DeprecatedClusterTagValueSeparator)
		if deprecatedVksClusterValue != "" {
			ensuredTags[domain.DeprecatedClusterTagKey] = deprecatedVksClusterValue
		}
	}

	// ignore delete VpcTagKey and BillingTagKey
	delete(createdTags, domain.VpcTagKey)
	delete(createdTags, domain.BillingTagKey)

	needUpdate, ensuredTags := t.buildTag(ctx, currentTags, createdTags, ensuredTags)
	if !needUpdate {
		return nil
	}

	t.logger.Infof("Updating tags for load balancer %s: %v", lbId, ensuredTags)
	err = t.vngcloudRepo.CreateTags(ctx, lbId, ensuredTags)
	if err != nil {
		return err
	}

	return t.statusAddCreatedTags(ctx, ensuredTags)
}
