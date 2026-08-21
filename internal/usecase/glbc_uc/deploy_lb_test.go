package glbc_uc

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

const oldGlbId = "glb-old"

// teardownOnLoadBalancer reclaims what this GlobalLoadBalancerConfig created on the load
// balancer it is moving away from. It must never delete that load balancer: it can be
// user-provided via spec.loadBalancerId and shared with other configs.
//
// The mock is strict, so any call not EXPECT()ed below fails the test - which is what
// pins the safety property: a DeleteGlobalLoadBalancer call would be unexpected.
func TestTeardownOnLoadBalancerNeverDeletesTheLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	vngcloudRepo.EXPECT().
		GetGlobalLoadBalancerByID(mock.Anything, oldGlbId).
		Return(&entityv2.GlobalLoadBalancer{ID: oldGlbId}, nil)
	vngcloudRepo.EXPECT().
		ListGlobalListeners(mock.Anything, oldGlbId).
		Return(&entityv2.ListGlobalListeners{Items: []*entityv2.GlobalListener{}}, nil)
	vngcloudRepo.EXPECT().
		ListGlobalPools(mock.Anything, oldGlbId).
		Return(&entityv2.ListGlobalPools{Items: []*entityv2.GlobalPool{}}, nil)

	var mutate func(context.Context, *v1alpha1.GlobalLoadBalancerConfig) bool
	k8sRepo.EXPECT().
		PatchMutateStatusGlobalLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ *v1alpha1.GlobalLoadBalancerConfig,
			f func(context.Context, *v1alpha1.GlobalLoadBalancerConfig) bool) error {
			mutate = f
			return nil
		})

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
			Status: v1alpha1.GlobalLoadBalancerConfigStatus{
				CreatedListeners: []v1alpha1.CreatedGlobalListener{},
				CreatedPools:     []v1alpha1.CreatedGlobalPool{},
			},
		},
	}

	require.NoError(t, task.teardownOnLoadBalancer(context.Background(), oldGlbId))

	require.NotNil(t, mutate, "status must be cleared after teardown")
	obj := &v1alpha1.GlobalLoadBalancerConfig{
		Status: v1alpha1.GlobalLoadBalancerConfigStatus{
			CreatedListeners: []v1alpha1.CreatedGlobalListener{{Id: "listener-1"}},
			CreatedPools:     []v1alpha1.CreatedGlobalPool{{Id: "pool-1", Name: "vks-a-b-80"}},
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
		GetGlobalLoadBalancerByID(mock.Anything, oldGlbId).
		Return(nil, errors.New("global_load_balancer_not_found"))

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		lbConfig:     &v1alpha1.GlobalLoadBalancerConfig{},
	}

	assert.NoError(t, task.teardownOnLoadBalancer(context.Background(), oldGlbId))
}
