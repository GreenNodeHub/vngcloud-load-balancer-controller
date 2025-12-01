package builder

import (
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
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
	vksClusterTags := currentTags[domain.ClusterTagKey]
	if !strings.Contains(vksClusterTags, r.fleetID) {
		r.logger.Debugf("Need update tag: %s", domain.ClusterTagKey)
		vksClusterTags = r.joinVKSTag(vksClusterTags, r.fleetID)
		newTags[domain.ClusterTagKey] = vksClusterTags
	}

	return r.updateTag(currentTags, oldTags, newTags)
}

func (r *vngcloudLBBuilder) updateTag(currentTags, oldTags, newTags map[string]string) error {
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
	if err := r.provider.CreateTags(r.context, r.loadBalancerID, mergeTags); err != nil {
		r.logger.Errorf("Failed to update tags: %v", err)
		return err
	}
	return nil
}

func (r *vngcloudLBBuilder) joinVKSTag(current, id string) string {
	tags := strings.Split(current, domain.ClusterTagValueSeparator)
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
	return strings.Join(newTags, domain.ClusterTagValueSeparator)
}
func (r *vngcloudLBBuilder) removeVKSTag(current, id string) string {
	tags := strings.Split(current, domain.ClusterTagValueSeparator)
	tagsValid := make(map[string]bool)
	for _, tag := range tags {
		if isValidVKSID(tag) {
			tagsValid[tag] = true
		} else {
			r.logger.Warnf("Invalid VKS cluster id tag: %s.", tag)
		}
	}
	if isValidVKSID(id) {
		delete(tagsValid, id)
	}
	newTags := make([]string, 0)
	for tag := range tagsValid {
		newTags = append(newTags, tag)
	}
	if len(newTags) == 0 {
		return ""
	}
	return strings.Join(newTags, domain.ClusterTagValueSeparator)
}

func isValidVKSID(id string) bool {
	return len(id) == domain.VKS_CLUSTER_ID_LENGTH && strings.HasPrefix(id, domain.VKS_CLUSTER_ID_PREFIX)
}

func (r *vngcloudLBBuilder) EnsureDeleteTags(oldBuilder OldModelBuilder) error {
	var (
		oldTags     = make(map[string]string)
		currentTags = make(map[string]string)
		newTags     = make(map[string]string)
	)
	if oldBuilder != nil && oldBuilder.GetOldTags() != nil {
		oldTags = oldBuilder.GetOldTags()
	}
	if r.tags != nil {
		currentTags = r.tags
	}

	// ensure have cluster ids tag
	vksClusterTags := currentTags[domain.ClusterTagKey]
	if strings.Contains(vksClusterTags, r.fleetID) {
		r.logger.Debugf("Need update tag: %s", domain.ClusterTagKey)
		vksClusterTags = r.removeVKSTag(vksClusterTags, r.fleetID)
		if vksClusterTags == "" {
			// remove tag
			oldTags[domain.ClusterTagKey] = currentTags[domain.ClusterTagKey]
		} else {
			newTags[domain.ClusterTagKey] = vksClusterTags
		}
	}

	return r.updateTag(currentTags, oldTags, newTags)
}
