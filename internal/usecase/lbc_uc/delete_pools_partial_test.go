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
)

// twoRedundantPools sets up a load balancer carrying two pools this LBC created, neither in
// use by a listener or a policy, so both are candidates for deletion.
func twoRedundantPools(vngcloud *repository.MockVngCloudRepository) *defaultModelDeployTask {
	vngcloud.EXPECT().
		ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{
			{UUID: "pool-1", Name: "vks-a-b-80"},
			{UUID: "pool-2", Name: "vks-a-b-81"},
		}}, nil)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	// no members anywhere, so canDeleteWholePool says the whole pool can go
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, "lb-1", mock.Anything).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil).Maybe()

	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer4},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo("lb-1"),
				CreatedPools: []v1alpha1.CreatedPool{
					{Id: "pool-1", Name: "vks-a-b-80"},
					{Id: "pool-2", Name: "vks-a-b-81"},
				},
			},
		},
	}
}

// One pool that will not go used to take the pools behind it with it: the loop returned at the
// first failure, and since the failure recurred every pass, the pools after it were never
// cleaned up. That is what happened on lb-87f329e4 - four pools untouched for a day because
// the first one kept answering "not ready".
func TestDeleteRedundantPoolsKeepsGoingAfterOnePoolFails(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	task := twoRedundantPools(vngcloud)

	deleted := make([]string, 0)
	vngcloud.EXPECT().
		DeletePool(mock.Anything, "lb-1", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, poolId string) error {
			if poolId == "pool-1" {
				// a real failure, not a busy load balancer, so it is not retried
				return errors.New("pool is referenced by something we cannot see")
			}
			deleted = append(deleted, poolId)
			return nil
		})
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()

	err := task.deleteRedundantPools(context.Background(), "lb-1", []v1alpha1.CreatedPool{})

	require.Error(t, err, "the load balancer is not in the state we asked for, so the reconcile must retry")
	assert.ErrorIs(t, err, errPartialDelete)
	assert.Contains(t, err.Error(), "pool-1")
	assert.Equal(t, []string{"pool-2"}, deleted,
		"the pool behind the failure must still be cleaned up")
}

// vLB rejects a write while it is still applying the previous one, and deleting several pools
// in a row is exactly the shape that provokes it: each delete pushes the load balancer into
// UPDATING and the next one arrives too early. Transient, so it is retried rather than counted
// as a failure - which is what left five pools dirty for a day.
func TestDeleteRedundantPoolsRetriesABusyLoadBalancer(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	task := twoRedundantPools(vngcloud)

	attempts := 0
	vngcloud.EXPECT().
		DeletePool(mock.Anything, "lb-1", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, poolId string) error {
			attempts++
			if poolId == "pool-1" && attempts == 1 {
				return notReadyErr()
			}
			return nil
		})
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil)

	err := task.deleteRedundantPools(context.Background(), "lb-1", []v1alpha1.CreatedPool{})

	assert.NoError(t, err, "a busy load balancer is transient, not a failure")
	assert.Equal(t, 3, attempts, "pool-1 is retried once after the wait, then pool-2 is deleted")
}
