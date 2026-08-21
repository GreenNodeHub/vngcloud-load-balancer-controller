package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/controller-runtime/pkg/client"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

const (
	// Two clusters sharing one load balancer. Both must survive in the cluster tag.
	thisClusterId  = "k8s-10eafaef-56e8-4dfc-878d-dd1c86fcb810"
	otherClusterId = "k8s-4bb03c1f-7463-46c1-8bfb-ca3fc16fb085"
	thirdClusterId = "k8s-6fa77e92-fa77-44ff-b5fa-e28f0c00b545"
)

func tagList(tags map[string]string) *entityv2.ListTags {
	items := make([]*entityv2.Tag, 0, len(tags))
	for k, v := range tags {
		items = append(items, &entityv2.Tag{Key: k, Value: v})
	}
	return &entityv2.ListTags{Items: items}
}

// expectNoOtherLBC makes the reference lookup on the delete path find nobody else using the
// load balancer.
func expectNoOtherLBC(k8sRepo *repository.MockK8sRepository) {
	k8sRepo.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything).
		Return(nil).
		Once()
}

// expectOtherLBC makes the lookup find a sibling LBC deployed on the same load balancer.
func expectOtherLBC(k8sRepo *repository.MockK8sRepository, lbId string) {
	k8sRepo.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, list *v1alpha1.LoadBalancerConfigList, _ ...client.ListOption) error {
			list.Items = []v1alpha1.LoadBalancerConfig{{
				ObjectMeta: metav1.ObjectMeta{Name: "sibling", Namespace: "default", UID: "other-uid"},
				Status:     v1alpha1.LoadBalancerConfigStatus{LoadBalancerId: ptr.To(lbId)},
			}}
			return nil
		}).
		Once()
}

func tagTask(vngcloudRepo *repository.MockVngCloudRepository, k8sRepo *repository.MockK8sRepository) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "lbc", Namespace: "default", UID: "self-uid"},
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId: ptr.To(thisClusterId),
				VpcId:     "net-11111111-2222-3333-4444-555555555555",
			},
		},
	}
}

// The steady state, and the reason the cache exists: everything already matches, so the
// reconcile must get away with a single read and no write at all. The mock is strict, so
// CreateTags and InvalidateTagsCache going undeclared is itself the assertion that neither
// is called.
func TestDeployTagsNoChangeReadsOnceAndWritesNothing(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(tagList(map[string]string{
			domain.ClusterTagKey: thisClusterId,
			domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
			domain.BillingTagKey: domain.BillingTagValue,
		}), nil).
		Times(1)

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))
}

// The bug the two-pass design exists to prevent. The first read is stale - it predates
// another cluster adding itself to the cluster tag - so a controller that wrote from it
// would erase that cluster's id. The write must be computed from the fresh read instead,
// and end up listing both clusters.
func TestDeployTagsWritesFromFreshReadNotTheCachedOne(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	staleTags := map[string]string{}
	freshTags := map[string]string{domain.ClusterTagKey: otherClusterId}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(staleTags), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(freshTags), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))

	// joinTagValue sorts, so the order is deterministic.
	assert.Equal(t, thisClusterId+domain.ClusterTagValueSeparator+otherClusterId,
		written[domain.ClusterTagKey],
		"the cluster tag must be merged from the fresh read, keeping the other cluster's id")
	assert.Equal(t, task.lbConfig.Spec.VpcId, written[domain.VpcTagKey])
	assert.Equal(t, domain.BillingTagValue, written[domain.BillingTagKey])
}

// If the fresh read shows the work was already done - another LBC on the same load balancer
// got there first in the same resync burst - there is nothing left to write.
func TestDeployTagsSkipsWriteWhenFreshReadIsAlreadyCorrect(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	done := map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(done), nil).Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))
}

// Nothing is written when the load balancer cannot be read, cached or not.
func TestDeployTagsPropagatesReadError(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(nil, assert.AnError).
		Once()

	assert.ErrorIs(t, task.deployTags(context.Background(), "lb-123"), assert.AnError)
}

// The removal path has the same hazard in mirror image: dropping this cluster's id from a
// stale list would take with it any id added since the list was read. Here a third cluster
// joined after the stale read, and must still be listed afterwards.
func TestDeleteRedundantTagsRemovesOnlyThisClusterFromFreshRead(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	sep := domain.ClusterTagValueSeparator
	stale := map[string]string{domain.ClusterTagKey: otherClusterId + sep + thisClusterId}
	fresh := map[string]string{domain.ClusterTagKey: otherClusterId + sep + thirdClusterId + sep + thisClusterId}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(stale), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(fresh), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
	assert.Equal(t, otherClusterId+sep+thirdClusterId, written[domain.ClusterTagKey],
		"only this cluster's id may be removed, and from the fresh list")
}

// Spec belongs to the Ingress/Service controller; the tag path must not scribble the
// cluster, VPC or billing tags into the object it was handed.
func TestDeployTagsDoesNotMutateSpecTags(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Spec.Tags = map[string]string{"team": "vks"}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, "lb-123", mock.Anything).Return(nil).Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))
	assert.Equal(t, map[string]string{"team": "vks"}, task.lbConfig.Spec.Tags)
}

// The #20 leak: when the last cluster stops using a load balancer that outlives it, its id
// has to come out of the cluster tag. Nothing noticed before, because the diff only looked
// at keys the wanted set still had - so the id stayed there for good.
func TestDeleteRedundantTagsRemovesTheClusterTagWhenTheLastClusterLeaves(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	onTheLoadBalancer := map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))

	assert.NotContains(t, written, domain.ClusterTagKey, "the departing cluster's id must not be left behind")
	// VpcTagKey and BillingTagKey are deliberately kept, so the write is not empty.
	assert.Equal(t, task.lbConfig.Spec.VpcId, written[domain.VpcTagKey])
	assert.Equal(t, domain.BillingTagValue, written[domain.BillingTagKey])
}

// CreateTags overwrites every tag on the load balancer, so anything the controller did not
// author - a tag set from the portal, say - has to be carried through the write untouched.
// Here another cluster is still using the load balancer, so a write does happen.
func TestDeleteRedundantTagsKeepsTagsItDidNotAuthor(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey: otherClusterId + domain.ClusterTagValueSeparator + thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	onTheLoadBalancer := map[string]string{
		"owner":              "platform-team",
		domain.ClusterTagKey: otherClusterId + domain.ClusterTagValueSeparator + thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
	assert.Equal(t, "platform-team", written["owner"], "a tag the controller never authored must survive the write")
	assert.Equal(t, otherClusterId, written[domain.ClusterTagKey])
}

// Status records what this cluster asked for, not everything the write happened to carry.
// Recording the whole set makes the controller claim tags it merely passed through, and the
// next reconcile would then treat them as its own to remove.
func TestDeployTagsRecordsOnlyTheTagsItAuthored(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	onTheLoadBalancer := map[string]string{"owner": "platform-team"}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()

	var recorded map[string]string
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ *v1alpha1.LoadBalancerConfig,
			mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			fresh := &v1alpha1.LoadBalancerConfig{}
			mutate(ctx, fresh)
			recorded = fresh.Status.CreatedTags
			return nil
		}).
		Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))

	assert.Equal(t, "platform-team", written["owner"], "the write carries the tag through")
	assert.NotContains(t, recorded, "owner", "but status must not claim a tag this cluster did not author")
	assert.Equal(t, thisClusterId, recorded[domain.ClusterTagKey])
	assert.Equal(t, domain.BillingTagValue, recorded[domain.BillingTagKey])
}

// Whether the API reads an empty tag list as "remove everything" is untested, and an error
// on the delete path blocks the LBC's finalizer. So when the wanted set comes out empty the
// tags are left alone - the strict mock asserts no write is attempted.
func TestDeleteRedundantTagsRefusesToWriteAnEmptyTagSet(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{domain.ClusterTagKey: thisClusterId}

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(tagList(map[string]string{domain.ClusterTagKey: thisClusterId}), nil).
		Times(1)

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
}

// The cluster id says "this cluster uses this load balancer", so it belongs to the cluster
// rather than to one LBC. While a sibling LBC is still deployed on the same load balancer it
// must stay put - and since CreateTags overwrites, taking it out early would also mean
// rewriting every other tag for no reason. The strict mock asserts no write happens.
func TestDeleteRedundantTagsKeepsTheClusterTagWhileAnotherLBCUsesTheLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectOtherLBC(k8sRepo, "lb-123")
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
		domain.BillingTagKey: domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(tagList(map[string]string{
			domain.ClusterTagKey: thisClusterId,
			domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
			domain.BillingTagKey: domain.BillingTagValue,
		}), nil).
		Times(1)

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
}

// Finding out who else uses the load balancer costs a list of every LBC in the cluster, so
// it is only worth asking when there is a cluster tag to take off. The strict mock has no
// ListLoadBalancerConfig expectation, which is the assertion.
func TestDeleteRedundantTagsDoesNotLookUpSiblingsWithoutAClusterTag(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(tagList(map[string]string{
			domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
			domain.BillingTagKey: domain.BillingTagValue,
		}), nil).
		Times(1)

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
}

// A lookup that fails must not be read as "nobody else uses it": the tag stays and the
// error is surfaced so the reconcile is retried.
func TestDeleteRedundantTagsKeepsTheClusterTagWhenTheLookupFails(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{domain.ClusterTagKey: thisClusterId}

	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, "lb-123").
		Return(tagList(map[string]string{
			domain.ClusterTagKey: thisClusterId,
			domain.VpcTagKey:     task.lbConfig.Spec.VpcId,
			domain.BillingTagKey: domain.BillingTagValue,
		}), nil).
		Times(1)
	k8sRepo.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything).
		Return(assert.AnError).
		Once()

	assert.ErrorIs(t, task.deleteRedundantTags(context.Background(), "lb-123"), assert.AnError)
}
