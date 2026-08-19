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
