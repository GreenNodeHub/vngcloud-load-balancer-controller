package vlbc_uc

import (
	"context"
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
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
	if t.vlbConfig.Spec.Tags != nil {
		ensuredTags = t.vlbConfig.Spec.Tags
	}
	createdTags := make(map[string]string)
	for k, v := range t.vlbConfig.Status.CreatedTags {
		createdTags[k] = v
	}

	if t.vlbConfig.Spec.ClusterId != nil && *t.vlbConfig.Spec.ClusterId != "" {
		// ensure have cluster ids tag
		vksClusterTags := currentTags[consts.VKS_TAG_KEY]
		if !strings.Contains(vksClusterTags, *t.vlbConfig.Spec.ClusterId) {
			t.logger.Debugf("Need update tag: %s", consts.VKS_TAG_KEY)
			vksClusterTags = t.joinVKSTag(vksClusterTags, *t.vlbConfig.Spec.ClusterId)
			ensuredTags[consts.VKS_TAG_KEY] = vksClusterTags
		}
	}

	if err := t.updateTag(ctx, lbId, currentTags, createdTags, ensuredTags); err != nil {
		return err
	}

	return t.k8sRepo.PatchMutateStatusVLBC(ctx, t.vlbConfig, func(ctx context.Context, obj *v1alpha1.VngcloudLoadBalancerConfig) {
		obj.Status.CreatedTags = ensuredTags
	})
}

func (r *defaultModelDeployTask) updateTag(ctx context.Context, lbId string, currentTags, oldTags, newTags map[string]string) error {
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
		return nil
	}

	r.logger.Infof("Need update tags: (%v) -> (%v)", currentTags, mergeTags)
	if err := r.vngcloudRepo.CreateTags(ctx, lbId, mergeTags); err != nil {
		r.logger.Errorf("Failed to update tags: %v", err)
		return err
	}
	return nil
}

func (r *defaultModelDeployTask) joinVKSTag(current, id string) string {
	tags := strings.Split(current, consts.VKS_TAGS_SEPARATOR)
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
	return strings.Join(newTags, consts.VKS_TAGS_SEPARATOR)
}

func isValidVKSID(id string) bool {
	return len(id) == consts.VKS_CLUSTER_ID_LENGTH && strings.HasPrefix(id, consts.VKS_CLUSTER_ID_PREFIX)
}
