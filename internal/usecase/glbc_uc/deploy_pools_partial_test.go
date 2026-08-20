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
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// Same defect as the regular load balancer path: returning on the first error abandoned
// every pool behind it, so whichever pool sat after a failure never got deployed in that
// pass - and the next pass starts at the first pool again.
func TestDeployPoolsKeepsGoingAfterOnePoolFails(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)

	vngcloud.EXPECT().
		ListGlobalPools(mock.Anything, "glb-1").
		Return(&entityv2.ListGlobalPools{Items: []*entityv2.GlobalPool{}}, nil)

	created := 0
	vngcloud.EXPECT().
		CreateGlobalPool(mock.Anything, "glb-1", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ global.ICreateGlobalPoolRequest) (*entityv2.GlobalPool, error) {
			created++
			if created == 2 {
				return nil, errors.New("pool protocol is invalid")
			}
			return &entityv2.GlobalPool{ID: "pool-ok", Name: "ok"}, nil
		}).Times(3)

	// the two pools that succeed each: record status, wait for the LB, then sync members
	k8s.EXPECT().
		PatchMutateStatusGlobalLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	vngcloud.EXPECT().
		WaitGlobalLoadBalancerActive(mock.Anything, "glb-1").
		Return(&entityv2.GlobalLoadBalancer{ID: "glb-1"}, nil).Times(2)
	vngcloud.EXPECT().
		ListGlobalPoolMembers(mock.Anything, "glb-1", "pool-ok").
		Return(&entityv2.ListGlobalPoolMembers{Items: []*entityv2.GlobalPoolMember{}}, nil).Times(2)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		cfg:          &config.Config{},
		lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
			Spec: v1alpha1.GlobalLoadBalancerConfigSpec{
				GlobalPools: []v1alpha1.GlobalPool{{Name: "p1"}, {Name: "p2"}, {Name: "p3"}},
			},
		},
	}

	pools, err := task.deployPools(context.Background(), "glb-1")

	assert.Equal(t, 3, created, "all three pools must be attempted, not just up to the failure")
	assert.Len(t, pools, 2, "the two pools that succeeded are returned")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errPartialDeploy))
}
