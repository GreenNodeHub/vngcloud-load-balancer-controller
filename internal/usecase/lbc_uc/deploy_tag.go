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

// tagDiff is what one read of a load balancer's tags tells us.
type tagDiff struct {
	// needUpdate is whether the load balancer's tags differ from what we want.
	needUpdate bool

	// write is the whole tag set to send. CreateTags overwrites, so this carries the tags
	// we do not manage too - dropping them here would delete them from the load balancer.
	write map[string]string

	// authored is only the tags this cluster asked for. It is what goes into status, so the
	// controller never claims - and later removes - a tag it merely carried over.
	authored map[string]string
}

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
	diff, err := t.diffTags(ctx, lbId, computeTags)
	if err != nil || !diff.needUpdate {
		return err
	}

	t.vngcloudRepo.InvalidateTagsCache(lbId)
	diff, err = t.diffTags(ctx, lbId, computeTags)
	if err != nil || !diff.needUpdate {
		return err
	}

	t.logger.Infof("Updating tags for load balancer %s: %v", lbId, diff.write)
	if err := t.vngcloudRepo.CreateTags(ctx, lbId, diff.write); err != nil {
		return err
	}

	return t.statusAddCreatedTags(ctx, diff.authored)
}

// diffTags reads the load balancer's tags and reports whether they differ from what
// computeTags wants, along with the set that would be written.
func (t *defaultModelDeployTask) diffTags(ctx context.Context, lbId string, computeTags computeTagsFunc) (tagDiff, error) {
	listTags, err := t.vngcloudRepo.ListTags(ctx, lbId)
	if err != nil {
		return tagDiff{}, err
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
	return tagDiff{needUpdate: needUpdate, write: mergedTags, authored: ensuredTags}, nil
}

// createdByThisCluster reports whether this cluster created the load balancer, which is what
// makes it ours to delete. A load balancer the user brought must never be deleted, however
// empty it looks to us: an Ingress deleted and recreated - an Argo replace, a helm reinstall -
// would otherwise take the user's load balancer and its address with it.
//
// Three signals, most trustworthy first:
//
//  1. The provenance tag, written by whichever LBC created the load balancer. This is the one
//     that outlives that LBC, which is what matters when several LBCs share a load balancer
//     and the one that created it is deleted first. A tag naming another cluster is a
//     definite no.
//  2. This LBC's own record of having created it, for the window between creating a load
//     balancer and getting the tag onto it.
//  3. For a load balancer that predates the tag: nothing here can prove provenance, so fall
//     back to what the user asked for. Pinning an id is the only sign we have that the load
//     balancer might not be ours, and leaving one behind is a great deal better than deleting
//     somebody else's.
//
// Note there is deliberately no comparison of names: Spec.LoadBalancerName is mirrored from
// the cloud once a load balancer is adopted, so it says nothing about who created it.
//
// currentTags comes from a read the caller already needed, so this costs nothing.
func (t *defaultModelDeployTask) createdByThisCluster(lbId string, currentTags map[string]string) bool {
	if createdBy, tagged := currentTags[domain.CreatedByClusterTagKey]; tagged {
		return t.lbConfig.Spec.ClusterId != nil && createdBy == *t.lbConfig.Spec.ClusterId
	}

	if t.lbConfig.Status.CreatedLoadBalancerId != nil && *t.lbConfig.Status.CreatedLoadBalancerId == lbId {
		return true
	}

	pinned := t.lbConfig.Spec.LoadBalancerId != nil && *t.lbConfig.Spec.LoadBalancerId != ""
	return !pinned
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
		// The tags we wrote last time. Any key here that is no longer wanted is dropped from
		// the write, which is how a tag the owner removed from its annotations comes off the
		// load balancer. Status therefore has to record only what this cluster asked for - see
		// ensureTags - or the controller would claim, and then delete, tags it merely carried
		// over from the portal or from another cluster.
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

		// Record on the load balancer that this cluster created it, so that once this LBC is
		// gone the others can still tell it apart from one the user brought. Only the LBC that
		// created it may assert this, and only from its own record - never from the fallbacks
		// in createdByThisCluster, which would let an adopted load balancer claim itself.
		if t.lbConfig.Spec.ClusterId != nil && isValidVksId(*t.lbConfig.Spec.ClusterId) &&
			t.lbConfig.Status.CreatedLoadBalancerId != nil && *t.lbConfig.Status.CreatedLoadBalancerId == lbId {
			ensuredTags[domain.CreatedByClusterTagKey] = *t.lbConfig.Spec.ClusterId
		}

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

	// CreateTags overwrites the whole tag set, so mergeTags is what the load balancer will
	// carry afterwards - and a key missing from it is one we are asking to have removed. The
	// comparison above only looks at keys mergeTags has, so nothing ever noticed a removal:
	// when the last cluster stopped using a load balancer that outlived it, its id stayed in
	// the cluster tag for good.
	//
	// Only the cluster tag is checked. That one is unambiguously ours, whereas acting on any
	// other missing key would delete a tag somebody set outside the controller.
	if !isNeedUpdate {
		if _, wanted := mergeTags[domain.ClusterTagKey]; !wanted {
			if current, present := currentTags[domain.ClusterTagKey]; present {
				r.logger.Infof("Tag diff: key=%s must be removed, current=%q", domain.ClusterTagKey, current)
				isNeedUpdate = true
			}
		}
	}

	if !isNeedUpdate {
		r.logger.Debug("No need update tags")
		return false, nil
	}

	// Never ask for an empty tag set. Whether the API takes that as "remove everything" is
	// untested, and an error on this path blocks the LBC's finalizer - leaving the tags
	// alone is the safe answer.
	if len(mergeTags) == 0 {
		r.logger.Warnf("Refusing to write an empty tag set: leaving tags %v in place", currentTags)
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
