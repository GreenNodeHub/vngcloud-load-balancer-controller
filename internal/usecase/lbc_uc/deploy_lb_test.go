package lbc_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

const oldLbId = "lb-old"

// teardownOnLoadBalancer reclaims what this LoadBalancerConfig created on the load
// balancer it is moving away from. It must never delete that load balancer: it can be
// user-provided via spec.loadBalancerId and shared with other LoadBalancerConfigs, so
// deleting it would take down unrelated traffic.
//
// The mock is strict, so any call that is not EXPECT()ed below fails the test. That is
// what pins the safety property: a DeleteLoadBalancer call would be unexpected.
func TestTeardownOnLoadBalancerNeverDeletesTheLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	vngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)
	// deleteRedundantListeners lists listeners of the OLD load balancer
	vngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, oldLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	// deleteRedundantPools lists pools of the OLD load balancer
	vngcloudRepo.EXPECT().
		ListPool(mock.Anything, oldLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
	// deleteRedundantTags reads its tags. Nothing here to give up, so nothing is written -
	// and with no cluster tag present it does not even ask who else uses the load balancer.
	vngcloudRepo.EXPECT().
		ListTags(mock.Anything, oldLbId).
		Return(&entityv2.ListTags{Items: []*entityv2.Tag{}}, nil)

	// capture the status mutation so we can assert it clears the recorded resources
	var mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ *v1alpha1.LoadBalancerConfig,
			f func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			mutate = f
			return nil
		})

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:   ptrTo(oldLbId),
				CreatedListeners: []v1alpha1.CreatedListener{},
				CreatedPools:     []v1alpha1.CreatedPool{},
			},
		},
	}

	require.NoError(t, task.teardownOnLoadBalancer(context.Background(), oldLbId))

	require.NotNil(t, mutate, "status must be cleared after teardown")
	obj := &v1alpha1.LoadBalancerConfig{
		Status: v1alpha1.LoadBalancerConfigStatus{
			CreatedListeners: []v1alpha1.CreatedListener{{Id: "listener-1"}},
			CreatedPools:     []v1alpha1.CreatedPool{{Id: "pool-1", Name: "vks-a-b-80"}},
		},
	}
	assert.True(t, mutate(context.Background(), obj), "clearing a non-empty status is a change")
	assert.Empty(t, obj.Status.CreatedPools,
		"stale pool entries would collide with the new load balancer's pools")
	assert.Empty(t, obj.Status.CreatedListeners)
	assert.Empty(t, obj.Status.CreatedTags,
		"tags recorded against the old load balancer would be treated as ours to drop on the new one")
}

// Migrating away is the one path that leaves a load balancer behind for good: status now
// points at the new one, so nothing ever reconciles the old one again. A cluster id left in
// vng.vks.cluster.ids there would stay for good, and the load balancer would go on looking
// like this cluster's.
func TestTeardownOnLoadBalancerTakesTheClusterTagOffTheOldLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	onTheOldLoadBalancer := map[string]string{
		domain.ClusterTagKey:          thisClusterId,
		domain.CreatedByClusterTagKey: thisClusterId,
		domain.VpcTagKey:              "net-1111",
		domain.BillingTagKey:          domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)
	vngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, oldLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloudRepo.EXPECT().
		ListPool(mock.Anything, oldLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)

	// nobody else in the cluster is using it, so the id is ours to take off
	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLoadBalancer), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache(oldLbId).Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLoadBalancer), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, oldLbId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{ClusterId: ptrTo(thisClusterId)},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(oldLbId),
				CreatedTags:    onTheOldLoadBalancer,
			},
		},
	}

	require.NoError(t, task.teardownOnLoadBalancer(context.Background(), oldLbId))

	assert.NotContains(t, written, domain.ClusterTagKey,
		"this cluster no longer uses the old load balancer, so its id must come off")
	assert.Equal(t, thisClusterId, written[domain.CreatedByClusterTagKey],
		"provenance is not ours to erase: this cluster did create it")
	assert.Equal(t, domain.BillingTagValue, written[domain.BillingTagKey])
}

// A load balancer that is already gone is not an error: there is nothing left to
// reclaim, and failing here would block the migration forever.
func TestTeardownOnLoadBalancerIgnoresMissingLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	vngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(nil, errors.New("Cannot get load balancer with id "+oldLbId))

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	assert.NoError(t, task.teardownOnLoadBalancer(context.Background(), oldLbId))
}

func ptrTo[T any](v T) *T { return &v }
