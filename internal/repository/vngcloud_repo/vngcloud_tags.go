package vngcloud_repo

import (
	"context"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// // --------------------------- Tags ---------------------------

const (
	// Tags only change when a cluster starts or stops using a load balancer, which is
	// rare, while the controller re-reads them on every reconcile of every LBC. Five
	// minutes is enough to collapse a resync burst - all the LBCs sharing one load
	// balancer wake together - into a single read, and is also the longest a tag edit made
	// outside the controller (say, from the portal) can go unnoticed.
	tagCacheTTL = 5 * time.Minute

	// One entry per load balancer the controller touches; generous next to any project.
	tagCacheMaxSize = 512
)

// ListTags returns the resource's tags, from cache when one is remembered.
//
// This was the single largest source of rate-limited requests: 1119 of 2464 observed 429s
// in one incident came from here, all of them re-reading tags that had not changed.
//
// A cached answer is only ever used to decide whether a write is needed. Callers that are
// about to write must call InvalidateTagsCache first and decide again on a fresh read,
// because the cluster tag holds a list of cluster ids merged from the current value, and
// writing a stale one would drop another cluster's id. See ensureTags.
func (m *vngCloudRepository) ListTags(ctx context.Context, resourceID string) (*entityv2.ListTags, error) {
	if cached, ok := m.tagCache.get(resourceID); ok {
		return listTagsFrom(cached), nil
	}

	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListTagsRequest(resourceID)
	tags, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListTags(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}

	m.tagCache.put(resourceID, tagValues(tags))
	return tags, nil
}

// InvalidateTagsCache forgets the tags remembered for resourceID, so the next ListTags
// reads through to the API.
func (m *vngCloudRepository) InvalidateTagsCache(resourceID string) {
	m.tagCache.invalidate(resourceID)
}

// tagValues copies the tags out of an SDK response. Entries are stored by value so a hit
// can never hand two callers the same mutable Tag.
func tagValues(tags *entityv2.ListTags) []entityv2.Tag {
	if tags == nil {
		return nil
	}
	out := make([]entityv2.Tag, 0, len(tags.Items))
	for _, tag := range tags.Items {
		if tag == nil {
			continue
		}
		out = append(out, *tag)
	}
	return out
}

// listTagsFrom rebuilds an SDK response from cached values, one fresh copy per hit.
func listTagsFrom(values []entityv2.Tag) *entityv2.ListTags {
	items := make([]*entityv2.Tag, 0, len(values))
	for i := range values {
		tag := values[i]
		items = append(items, &tag)
	}
	return &entityv2.ListTags{Items: items}
}

func (m *vngCloudRepository) CreateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	// The tags are about to change; a failed call can still have applied part of it, so
	// drop what we remembered either way.
	defer m.tagCache.invalidate(resourceID)

	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create tags for resource %s", resourceID)
	opt := loadbalancerv2.NewCreateTagsRequest(resourceID)
	arr := make([]string, 0)
	for k, v := range tags {
		arr = append(arr, k)
		arr = append(arr, v)
	}
	opt.WithTags(arr...)

	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateTags(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}

func (m *vngCloudRepository) UpdateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	// The tags are about to change; a failed call can still have applied part of it, so
	// drop what we remembered either way.
	defer m.tagCache.invalidate(resourceID)

	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request update tags for resource %s", resourceID)
	opt := loadbalancerv2.NewUpdateTagsRequest(resourceID)
	arr := make([]string, 0)
	for k, v := range tags {
		arr = append(arr, k)
		arr = append(arr, v)
	}
	opt.WithTags(arr...)

	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdateTags(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("UpdateTags: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}
