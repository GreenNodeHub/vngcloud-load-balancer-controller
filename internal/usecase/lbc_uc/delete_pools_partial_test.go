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

// #33308 - a Service pinned to a customer's Layer 4 load balancer left this cluster's pool on it
// for good. The teardown restored the adopted listener's default pool correctly, and then never
// deleted the pool it had displaced.
//
// The asymmetry that causes it: a listener this LBC created is deleted and then waited on
// (WaitForLBActive), while an adopted listener is handed back with an UpdateListener and no wait
// at all. deleteRedundantPools runs straight after and starts by listing the listeners to see
// which pools are still in use - and in that window vLB is still UPDATING, so it answers with the
// default pool from before the restore.
//
// That misreading goes both ways. This cluster's pool reads as "in use" and is spared - the leak
// the issue reports. A pool of the customer's that deployPool adopted by name reads as used by
// nothing and is deleted, which is worse; the fixture below puts one in status.createdPools,
// which is how adopt-by-name records it.
//
// There is no second chance at either. A pool still read as in-use simply falls through both
// branches of the loop and is not recorded as a failure, so the teardown returns nil, the
// finalizer comes off and the LBC is gone: the first pass is the only pass.
//
// The mock models the one property that matters: the restored value becomes visible only once
// the load balancer has settled since the write.
func TestAdoptedListenerTeardownDeletesTheClusterPoolItDisplaced(t *testing.T) {
	const (
		lb       = "lb-1"
		listener = "lis-1"
		vksPool  = "pool-vks"
		custPool = "pool-customer"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)

	var restored, settled bool

	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		RunAndReturn(func(context.Context, string) (*entityv2.ListListeners, error) {
			defaultPool := vksPool
			if restored && settled {
				defaultPool = custPool
			}
			return &entityv2.ListListeners{Items: []*entityv2.Listener{
				{UUID: listener, DefaultPoolId: defaultPool},
			}}, nil
		})

	vngcloud.EXPECT().
		UpdateListener(mock.Anything, lb, listener, mock.Anything).
		RunAndReturn(func(context.Context, string, string, loadbalancerv2.IUpdateListenerRequest) error {
			restored = true
			settled = false // the write leaves the load balancer UPDATING
			return nil
		})

	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, lb).
		RunAndReturn(func(context.Context, string) (*entityv2.LoadBalancer, error) {
			settled = true
			return &entityv2.LoadBalancer{UUID: lb}, nil
		}).Maybe()

	vngcloud.EXPECT().
		ListPool(mock.Anything, lb).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{
			{UUID: vksPool, Name: "vks-a-b-TCP-8080"},
			{UUID: custPool, Name: "customer_pool_8080"},
		}}, nil)

	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lb, mock.Anything).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil).Maybe()

	deleted := make([]string, 0)
	vngcloud.EXPECT().
		DeletePool(mock.Anything, lb, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, poolId string) error {
			deleted = append(deleted, poolId)
			return nil
		}).Maybe()

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer4},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedListeners: []v1alpha1.CreatedListener{{
					Id:                    listener,
					Port:                  8080,
					Adopted:               true,
					OriginalDefaultPoolId: ptrTo(custPool),
				}},
				// Both pools are in the record on purpose. CreatedPool carries no provenance -
				// unlike CreatedListener there is no Adopted flag - so the teardown cannot tell
				// a pool it created from one it merely recorded, and every entry here is a
				// candidate. The only thing keeping the customer's is that a listener still
				// points at it, which is exactly the reading this change alters. With just our
				// own pool listed the assertion below is vacuous: the customer's could never be
				// a candidate whatever the code did.
				//
				// The name here is the customer's own, not one this controller would generate.
				// deployPool only records a pool it did not create when the names match exactly,
				// so this fixture models the recorded state rather than the route into it; the
				// deletion path compares ids and listener references and never looks at names.
				CreatedPools: []v1alpha1.CreatedPool{
					{Id: vksPool, Name: "vks-a-b-TCP-8080"},
					{Id: custPool, Name: "customer_pool_8080"},
				},
			},
		},
	}

	// The order delete() uses: listeners first, then the pools they no longer hold.
	ctx := context.Background()
	require.NoError(t, task.deleteRedundantListeners(ctx, lb, nil, nil))
	require.NoError(t, task.deleteRedundantPools(ctx, lb, nil))

	assert.True(t, restored, "the adopted listener must get its original default pool back")
	assert.Equal(t, []string{vksPool}, deleted,
		"the pool this cluster created must be deleted, and the customer's left alone")
	// Read the failure in both directions. Empty means the leak this issue is about. Holding
	// custPool means the worse half of the same stale read: with the listener still reported as
	// pointing at our pool, the customer's pool reads as used by nothing and is deleted.
	assert.NotContains(t, deleted, custPool,
		"the customer's own pool must never be deleted, however it got into the record")
}

// The wait above has to be conditional. An adopted listener that already carries its original
// default pool - a second teardown pass, or one that never displaced anything - writes nothing,
// and there is then nothing to become visible. Waiting anyway would be a new way to fail on a
// load balancer that is not ours: one its owner has left non-ACTIVE would hold a reconcile
// worker for the whole backoff, or come back ERROR and fail the teardown, which leaves the
// finalizer on and wedges the delete. The teardown used to walk straight past that case.
//
// The mock is strict, so the guard is that neither UpdateListener nor WaitForLBActive is
// declared here: either call fails the test.
func TestAdoptedListenerTeardownDoesNotWaitWhenItRestoredNothing(t *testing.T) {
	const (
		lb       = "lb-1"
		listener = "lis-1"
		custPool = "pool-customer"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{
			{UUID: listener, DefaultPoolId: custPool}, // already what it was adopted with
		}}, nil)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer4},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedListeners: []v1alpha1.CreatedListener{{
					Id:                    listener,
					Port:                  8080,
					Adopted:               true,
					OriginalDefaultPoolId: ptrTo(custPool),
				}},
			},
		},
	}

	require.NoError(t, task.deleteRedundantListeners(context.Background(), lb, nil, nil))
}
