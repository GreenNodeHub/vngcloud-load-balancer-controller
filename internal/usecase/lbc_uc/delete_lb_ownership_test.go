package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

const ownershipClusterId = "k8s-10eafaef-56e8-4dfc-878d-dd1c86fcb810"

// emptyLoadBalancer mocks the reads for a load balancer with nothing on it, so
// canDeleteWholeLoadBalancer says yes and every delete decision comes down to provenance.
// tags is what the load balancer carries, which is where provenance is read from.
func emptyLoadBalancer(vngcloud *repository.MockVngCloudRepository, tags map[string]string) {
	items := make([]*entityv2.Tag, 0, len(tags))
	for k, v := range tags {
		items = append(items, &entityv2.Tag{Key: k, Value: v})
	}
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil).Maybe()
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").
		Return(&entityv2.ListTags{Items: items}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).
		Return(nil).Maybe()
	vngcloud.EXPECT().InvalidateTagsCache("lb-1").Maybe()
}

// specLbId pins a load balancer to this LBC; createdLbId is this LBC's own record of having
// created one.
func ownershipTask(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository,
	specLbId, createdLbId *string) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				Type:           loadbalancerv2.LoadBalancerTypeLayer7,
				LoadBalancerId: specLbId,
				ClusterId:      ptr.To(ownershipClusterId),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:        ptr.To("lb-1"),
				CreatedLoadBalancerId: createdLbId,
			},
		},
	}
}

// A load balancer the user created is not ours to delete, however empty it looks: it may
// still serve workloads outside this cluster entirely. A shared ALB was destroyed this way
// once, taking its address with it and leaving every service on it dark until someone
// repointed them by hand.
//
// The mock is strict, so not expecting DeleteLoadBalancer is the assertion.
func TestDeleteLoadBalancerNeverDeletesOneTheUserCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	// Pinned by annotation, and nothing anywhere says the controller created it.
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// The ordinary case: this LBC created the load balancer, so it goes when the LBC does.
func TestDeleteLoadBalancerDeletesOneThisClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	task := ownershipTask(vngcloud, k8s, nil, ptr.To("lb-1"))

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// The case an annotation-based rule gets wrong. Several Services can share one load balancer
// by pinning its id, and the one that created it may be deleted first - after which the
// remaining LBCs look, by their Spec alone, exactly like a user who brought their own load
// balancer. The provenance tag is what tells them apart, and it says the cluster created this
// one, so the last LBC out still cleans it up.
func TestDeleteLoadBalancerDeletesAPinnedOneTheClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{domain.CreatedByClusterTagKey: ownershipClusterId})
	// Pinned by annotation, and this LBC is not the one that created it.
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// A load balancer another cluster created is not ours either, whatever our Spec looks like.
func TestDeleteLoadBalancerNeverDeletesOneAnotherClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.CreatedByClusterTagKey: "k8s-4bb03c1f-7463-46c1-8bfb-ca3fc16fb085",
	})
	task := ownershipTask(vngcloud, k8s, nil, nil)

	assert.NoError(t, task.delete(context.Background()))
}

// A load balancer from before the provenance tag existed, with nothing pinning it: there is
// no evidence either way, and this is the shape of every load balancer the controller has
// ever created on its own. Deleting it is the behaviour that was there before this guard, and
// changing it would leave load balancers behind on every existing cluster.
func TestDeleteLoadBalancerDeletesAnUnpinnedOneWithNoProvenance(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	task := ownershipTask(vngcloud, k8s, nil, nil)

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// Provenance has to be recorded while the load balancer is being deployed, because by the
// time it matters the LBC that created it may be gone.
func TestDeployTagsRecordsProvenanceOnALoadBalancerThisClusterCreated(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Status.CreatedLoadBalancerId = ptr.To("lb-123")

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()

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
	assert.Equal(t, thisClusterId, written[domain.CreatedByClusterTagKey])
}

// Only the cluster that created a load balancer may claim it. Adopting one must not.
func TestDeployTagsDoesNotClaimALoadBalancerItAdopted(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Spec.LoadBalancerId = ptr.To("lb-123")

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()

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
	assert.NotContains(t, written, domain.CreatedByClusterTagKey)
}

// The provenance tag has to outlive the LBC that wrote it, so the delete path must leave it
// alone even while taking this cluster's id out of the cluster tag.
func TestDeleteRedundantTagsKeepsTheProvenanceTag(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey:          thisClusterId,
		domain.CreatedByClusterTagKey: thisClusterId,
		domain.VpcTagKey:              task.lbConfig.Spec.VpcId,
		domain.BillingTagKey:          domain.BillingTagValue,
	}

	onTheLoadBalancer := map[string]string{
		domain.ClusterTagKey:          thisClusterId,
		domain.CreatedByClusterTagKey: thisClusterId,
		domain.VpcTagKey:              task.lbConfig.Spec.VpcId,
		domain.BillingTagKey:          domain.BillingTagValue,
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
	assert.NotContains(t, written, domain.ClusterTagKey, "the departing cluster's id must go")
	assert.Equal(t, thisClusterId, written[domain.CreatedByClusterTagKey], "but provenance must stay")
}
