package lbc_uc

import (
	"context"
	"slices"
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// oldTags: obj.Status.Tags
// newTags: obj.Spec.Tags
// currentTags: get in portal
// merge them and update
// if spec.cluster exist, add tag cluster ids
func (t *defaultModelDeployTask) deployTags(ctx context.Context, lbId string) error {
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
	if t.lbConfig.Spec.Tags != nil {
		ensuredTags = t.lbConfig.Spec.Tags
	}
	createdTags := make(map[string]string)
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
	isNeedUpdate := false
	if len(mergeTags) != len(currentTags) {
		isNeedUpdate = true
	}
	if !isNeedUpdate {
		for k, v := range mergeTags {
			if currentTags[k] != v {
				isNeedUpdate = true
			}
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
