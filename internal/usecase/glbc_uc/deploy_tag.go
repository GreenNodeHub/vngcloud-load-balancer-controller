package glbc_uc

import (
	"context"
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// oldTags: obj.Status.Tags
// newTags: obj.Spec.Tags
// currentTags: get in portal
// merge them and update
// if spec.cluster exist, add tag vks_cluster_ids
func (t *defaultModelDeployTask) deployTags(ctx context.Context, lbId string) error {
	currentTags := make(map[string]string)
	listTags, err := t.vngcloudRepo.ListTags(ctx, lbId)
	if err != nil {
		return err
	}
	for _, tag := range listTags.Items {
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

	needUpdate, ensuredTags := t.buildTag(ctx, currentTags, createdTags, ensuredTags)
	if !needUpdate {
		return nil
	}

	t.logger.Infof("Updating tags for load balancer %s: %v", lbId, ensuredTags)
	err = t.vngcloudRepo.CreateTags(ctx, lbId, ensuredTags)
	if err != nil {
		return err
	}

	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		obj.Status.CreatedTags = ensuredTags
	})
}

func (r *defaultModelDeployTask) buildTag(_ context.Context, currentTags, oldTags, newTags map[string]string) (bool, map[string]string) {
	r.logger.Debug("EnsureTags: ")
	r.logger.Debugf("   - oldTags:   %v", oldTags)
	r.logger.Debugf("   - curTags:   %v", currentTags)
	r.logger.Debugf("   - newTags:   %v", newTags)

	// ensure vks_cluster_ids tag
	if r.lbConfig.Spec.ClusterId != nil && *r.lbConfig.Spec.ClusterId != "" {
		// ensure have cluster ids tag
		vksClusterTags := currentTags[domain.VKS_TAG_KEY]
		if !strings.Contains(vksClusterTags, *r.lbConfig.Spec.ClusterId) {
			r.logger.Debugf("Need update tag: %s", domain.VKS_TAG_KEY)
			vksClusterTags = r.joinVKSTag(vksClusterTags, *r.lbConfig.Spec.ClusterId)
			newTags[domain.VKS_TAG_KEY] = vksClusterTags
		} else {
			newTags[domain.VKS_TAG_KEY] = vksClusterTags
		}
	}

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

	return true, mergeTags
}

func (r *defaultModelDeployTask) joinVKSTag(current, id string) string {
	tags := strings.Split(current, domain.VKS_TAGS_SEPARATOR)
	tagsValid := make(map[string]bool)
	for _, tag := range tags {
		if isValidVKSID(tag) {
			tagsValid[tag] = true
		} else {
			r.logger.Warnf("Invalid VKS cluster id tag: %s.", tag)
		}
	}
	if isValidVKSID(id) {
		tagsValid[id] = true
	}
	newTags := make([]string, 0)
	for tag := range tagsValid {
		newTags = append(newTags, tag)
	}
	return strings.Join(newTags, domain.VKS_TAGS_SEPARATOR)
}

func isValidVKSID(id string) bool {
	return len(id) == domain.VKS_CLUSTER_ID_LENGTH && strings.HasPrefix(id, domain.VKS_CLUSTER_ID_PREFIX)
}
