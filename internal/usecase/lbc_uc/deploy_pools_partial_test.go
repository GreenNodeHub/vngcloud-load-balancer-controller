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
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// One pool failing used to abandon the pools behind it, so whichever pool sat after a
// failure never got deployed in that pass - and since the failure recurred every pass, it
// could stay that way indefinitely. Each pool is independent, so the loop keeps going and
// reports what it could not do at the end.
func TestDeployPoolsKeepsGoingAfterOnePoolFails(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)

	// nothing exists on the load balancer yet, so every pool takes the create path
	vngcloud.EXPECT().
		ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)

	created := 0
	vngcloud.EXPECT().
		CreatePool(mock.Anything, "lb-1", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
			created++
			if created == 2 {
				// not a busy load balancer - a real failure, so it is not retried
				return nil, errors.New("pool protocol is invalid")
			}
			return &entityv2.Pool{UUID: "pool-ok", Name: "ok"}, nil
		}).Times(3)

	// the two pools that succeed each record themselves and wait for the load balancer
	k8s.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Times(2)
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Times(2)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		cfg:          &config.Config{},
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				Pools: []v1alpha1.Pool{{Name: "p1"}, {Name: "p2"}, {Name: "p3"}},
			},
		},
	}

	pools, err := task.deployPools(context.Background(), "lb-1")

	assert.Equal(t, 3, created, "all three pools must be attempted, not just up to the failure")
	assert.Len(t, pools, 2, "the two pools that succeeded are returned")
	require.Error(t, err, "the pass still reports failure so it gets requeued")
	assert.True(t, errors.Is(err, errPartialDeploy),
		"the caller distinguishes a partial pass from a total one via errPartialDeploy")
}
