package builder

import (
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

// Tag Key   required value must length from 3 to 255
// Tag Value required value must length from 3 to 255

func (r *vngcloudLBBuilder) EnsureTags(tags map[string]string, oldBuilder OldModelBuilder) error {
	var (
		oldTags     = make(map[string]string)
		currentTags = make(map[string]string)
		newTags     = make(map[string]string)
	)
	if tags != nil {
		newTags = tags
	}
	if oldBuilder != nil && oldBuilder.GetOldTags() != nil {
		oldTags = oldBuilder.GetOldTags()
	}
	if r.tags != nil {
		currentTags = r.tags
	}

	// ensure have cluster ids tag
	vksClusterTags := currentTags[consts.VKS_TAG_KEY]
	if !strings.Contains(vksClusterTags, r.clusterID) {
		r.logger.Debugf("Need update tag: %s", consts.VKS_TAG_KEY)
		vksClusterTags = joinVKSTag(vksClusterTags, r.clusterID)
		newTags[consts.VKS_TAG_KEY] = vksClusterTags
	}

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
		r.logger.Debugf("Need update tags: %v, (%d) -> (%d)", mergeTags, len(currentTags), len(mergeTags))
		isNeedUpdate = true
	}
	for k, v := range mergeTags {
		if currentTags[k] != v {
			r.logger.Debugf("Need update tag %s: (%s -> %s)", k, currentTags[k], v)
			isNeedUpdate = true
		}
	}

	if !isNeedUpdate {
		r.logger.Debug("No need update tags")
		return nil
	}

	r.logger.Debugf("Update tags: %v", mergeTags)
	if err := r.provider.CreateTags(r.context, r.loadBalancerID, mergeTags); err != nil {
		r.logger.Errorf("Failed to update tags: %v", err)
		return err
	}
	return nil
}

func joinVKSTag(current, id string) string {
	tags := strings.Split(current, consts.VKS_TAGS_SEPARATOR)
	tagsValid := make(map[string]bool)
	for _, tag := range tags {
		if isValidVKSID(tag) {
			tagsValid[tag] = true
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
