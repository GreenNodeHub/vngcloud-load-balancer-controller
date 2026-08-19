package lbc_uc

import (
	"context"
	"slices"
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// computeTagsFunc produces the tags this cluster wants on a load balancer, given the tags
// the load balancer currently carries. It also returns the tags this cluster created
// previously, which buildTag treats as ours to drop.
//
// It is a function rather than a value because ensureTags may have to compute the answer
// twice, against two different reads.
type computeTagsFunc func(currentTags map[string]string) (ensured, created map[string]string)

// ensureTags brings a load balancer's tags to what computeTags asks for, writing only if
// they differ.
//
// The first read may be served from cache (see ListTags), which is the point: in the
// steady state nothing needs changing and that read is pure overhead on every reconcile of
// every LBC. It was the largest single source of rate-limited requests.
//
// A cached read is only ever trusted to say "nothing to do". Before writing we invalidate
// and decide again on a fresh read, because the cluster tag holds a list of cluster ids
// merged from the value we just saw: writing a stale one would silently drop the id of
// another cluster that started using this load balancer meanwhile. Writes are rare, so the
// extra read costs almost nothing.
//
// The cost of a stale read is therefore only a missed update - a tag edited outside the
// controller heals on a later reconcile, at most one TTL later - never a wrong write.
func (t *defaultModelDeployTask) ensureTags(ctx context.Context, lbId string, computeTags computeTagsFunc) error {
	needUpdate, _, err := t.diffTags(ctx, lbId, computeTags)
	if err != nil || !needUpdate {
		return err
	}

	t.vngcloudRepo.InvalidateTagsCache(lbId)
	needUpdate, ensuredTags, err := t.diffTags(ctx, lbId, computeTags)
	if err != nil || !needUpdate {
		return err
	}

	t.logger.Infof("Updating tags for load balancer %s: %v", lbId, ensuredTags)
	if err := t.vngcloudRepo.CreateTags(ctx, lbId, ensuredTags); err != nil {
		return err
	}

	return t.statusAddCreatedTags(ctx, ensuredTags)
}

// diffTags reads the load balancer's tags and reports whether they differ from what
// computeTags wants, along with the set that would be written.
func (t *defaultModelDeployTask) diffTags(ctx context.Context, lbId string, computeTags computeTagsFunc) (bool, map[string]string, error) {
	listTags, err := t.vngcloudRepo.ListTags(ctx, lbId)
	if err != nil {
		return false, nil, err
	}
	currentTags := make(map[string]string)
	for _, tag := range listTags.Items {
		// ignore system tag, not allow to modify
		if tag.SystemTag {
			continue
		}
		currentTags[tag.Key] = tag.Value
	}

	ensuredTags, createdTags := computeTags(currentTags)
	needUpdate, mergedTags := t.buildTag(ctx, currentTags, createdTags, ensuredTags)
	return needUpdate, mergedTags, nil
}

// deployTags ensures the tags this cluster owns are set on the load balancer: the cluster
// id (appended to whatever other clusters are already listed), the VPC id, and billing.
func (t *defaultModelDeployTask) deployTags(ctx context.Context, lbId string) error {
	return t.ensureTags(ctx, lbId, func(currentTags map[string]string) (map[string]string, map[string]string) {
		// Copy rather than build on Spec.Tags: Spec belongs to the owner controller, and
		// this runs more than once per ensureTags.
		ensuredTags := make(map[string]string, len(t.lbConfig.Spec.Tags))
		for k, v := range t.lbConfig.Spec.Tags {
			ensuredTags[k] = v
		}
		createdTags := make(map[string]string, len(t.lbConfig.Status.CreatedTags))
		for k, v := range t.lbConfig.Status.CreatedTags {
			createdTags[k] = v
		}

		// ensure ClusterTagKey tag and DeprecatedClusterTagKey tag
		if t.lbConfig.Spec.ClusterId != nil && isValidVksId(*t.lbConfig.Spec.ClusterId) {
			// ensure have ClusterTagKey
			vksClusterValue := currentTags[domain.ClusterTagKey]
			vksClusterValue = joinTagValue(vksClusterValue, *t.lbConfig.Spec.ClusterId, domain.ClusterTagValueSeparator)
			ensuredTags[domain.ClusterTagKey] = vksClusterValue

			// // ensure have DeprecatedClusterTagKey
			// deprecatedVksClusterValue := currentTags[domain.DeprecatedClusterTagKey]
			// deprecatedVksClusterValue = joinTagValue(deprecatedVksClusterValue, *t.lbConfig.Spec.ClusterId, domain.DeprecatedClusterTagValueSeparator)
			// ensuredTags[domain.DeprecatedClusterTagKey] = deprecatedVksClusterValue
		}

		// ensure vpc id tag
		if t.lbConfig.Spec.VpcId != "" {
			ensuredTags[domain.VpcTagKey] = t.lbConfig.Spec.VpcId
		}

		// ensure billing tag
		ensuredTags[domain.BillingTagKey] = domain.BillingTagValue

		return ensuredTags, createdTags
	})
}

func (r *defaultModelDeployTask) buildTag(_ context.Context, currentTags, oldTags, newTags map[string]string) (bool, map[string]string) {
	r.logger.Debug("EnsureTags: ")
	r.logger.Debugf("   - oldTags:   %v", oldTags)
	r.logger.Debugf("   - curTags:   %v", currentTags)
	r.logger.Debugf("   - newTags:   %v", newTags)

	// merge tags
	mergeTags := make(map[string]string)
	for k, v := range currentTags {
		if len(k) < 3 || len(k) > 255 {
			r.logger.Warnf("Tag key \"%s\" required value must length from 3 to 255", k)
			continue
		}
		if len(v) < 3 || len(v) > 255 {
			r.logger.Warnf("Tag value \"%s\" required value must length from 3 to 255", v)
			continue
		}
		mergeTags[k] = v
	}
	for k := range oldTags {
		delete(mergeTags, k)
	}
	for k, v := range newTags {
		if len(k) < 3 || len(k) > 255 {
			r.logger.Warnf("Tag key \"%s\" required value must length from 3 to 255", k)
			continue
		}
		if len(v) < 3 || len(v) > 255 {
			r.logger.Warnf("Tag value \"%s\" required value must length from 3 to 255", v)
			continue
		}
		mergeTags[k] = v
	}

	// compare and update tags
	// Only check if the tags we want to set are different from current
	// Don't compare lengths because portal may have extra tags we don't manage
	isNeedUpdate := false
	for k, v := range mergeTags {
		if !strings.EqualFold(currentTags[k], v) {
			r.logger.Infof("Tag diff: key=%s, current=%q, wanted=%q", k, currentTags[k], v)
			isNeedUpdate = true
			break
		}
	}

	if !isNeedUpdate {
		r.logger.Debug("No need update tags")
		return false, nil
	}

	r.logger.Debugf("   - mergeTags:   %v", mergeTags)
	return true, mergeTags
}

// joinTagValue adds a value to a separated string, avoiding duplicates.
// Example: joinTagValue("a/b", "c", "/") returns "a/b/c"
// Example: joinTagValue("a/b", "a", "/") returns "a/b" (no duplicate)
func joinTagValue(current, value, separator string) string {
	values := make(map[string]bool)
	for _, v := range strings.Split(current, separator) {
		if v != "" {
			values[v] = true
		}
	}
	if value != "" {
		values[value] = true
	}
	result := make([]string, 0, len(values))
	for v := range values {
		result = append(result, v)
	}
	slices.Sort(result) // Sort to ensure deterministic output
	return strings.Join(result, separator)
}

// removeTagValue removes a value from a separated string.
// Example: removeTagValue("a/b/c", "b", "/") returns "a/c"
// Example: removeTagValue("a/b", "x", "/") returns "a/b" (value not found)
func removeTagValue(current, value, separator string) string {
	result := make([]string, 0)
	for _, v := range strings.Split(current, separator) {
		if v != "" && v != value {
			result = append(result, v)
		}
	}
	slices.Sort(result) // Sort to ensure deterministic output
	return strings.Join(result, separator)
}

func isValidVksId(id string) bool {
	return len(id) == domain.VKS_CLUSTER_ID_LENGTH && strings.HasPrefix(id, domain.VKS_CLUSTER_ID_PREFIX)
}
