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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// #33309 - a pool of the user's that happens to carry the name this controller generates was
// matched by name, written to status.createdPools, and then deleted by the teardown. The word
// "created" in that list is what does it: deleteRedundantPoolsFrom takes every entry as a
// deletion candidate, unconditionally.
//
// QC's own contrast is the proof that the teardown is otherwise right: a pool named anything else
// is never touched. What was wrong was how "mine" is decided, not what is done with it.
func TestDeleteLeavesAPoolThatWasOnlyAdopted(t *testing.T) {
	const (
		lb         = "lb-user"
		oursPool   = "pool-ours"
		theirsPool = "pool-theirs"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)
	vngcloud.EXPECT().
		ListPool(mock.Anything, lb).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{
			{UUID: oursPool, Name: "vks-a-b-80"},
			{UUID: theirsPool, Name: "vks-a-b-81"}, // the user's, carrying a name we generate
		}}, nil)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lb, mock.Anything).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil).Maybe()
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, lb).
		Return(&entityv2.LoadBalancer{UUID: lb}, nil).Maybe()

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
		k8sRepo:      noOtherLBC(t),
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer7},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedPools: []v1alpha1.CreatedPool{
					{Id: oursPool, Name: "vks-a-b-80"},
					{Id: theirsPool, Name: "vks-a-b-81", Adopted: true},
				},
			},
		},
	}

	require.NoError(t, task.deleteRedundantPools(context.Background(), lb, nil))

	assert.Contains(t, deleted, oursPool, "a pool this cluster created is still ours to remove")
	assert.NotContains(t, deleted, theirsPool,
		"a pool we only adopted by name belongs to the user and must be left on the load balancer")
}

// Leaving the pool is only half of handing it back. The members this controller put into a pool
// of the user's are still its own to take out - and that is exactly the case the teardown used to
// skip, because when every member in the pool is one of ours canDeleteWholePool answers "the whole
// pool can go" and builds no member update at all. Blocking the delete there and doing nothing
// else leaves our members in a stranger's pool for good, which is the mirror of #33308.
func TestDeleteTakesOurMembersBackOutOfAnAdoptedPool(t *testing.T) {
	const (
		lb         = "lb-user"
		theirsPool = "pool-theirs"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)
	vngcloud.EXPECT().
		ListPool(mock.Anything, lb).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: theirsPool, Name: "vks-a-b-80"}}}, nil)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	// every member in the pool is one this cluster added
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lb, theirsPool).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{
			{Address: "10.0.0.1", ProtocolPort: 80},
			{Address: "10.0.0.2", ProtocolPort: 80},
		}}, nil)
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, lb).
		Return(&entityv2.LoadBalancer{UUID: lb}, nil).Maybe()

	var handedBack loadbalancerv2.IUpdatePoolMembersRequest
	vngcloud.EXPECT().
		UpdatePoolMembers(mock.Anything, lb, theirsPool, mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, req loadbalancerv2.IUpdatePoolMembersRequest) error {
			handedBack = req
			return nil
		}).Once()

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      noOtherLBC(t),
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer7},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedPools: []v1alpha1.CreatedPool{{
					Id:      theirsPool,
					Name:    "vks-a-b-80",
					Adopted: true,
					CreatedMembers: []v1alpha1.PoolMember{
						{IP: "10.0.0.1", Port: 80},
						{IP: "10.0.0.2", Port: 80},
					},
				}},
			},
		},
	}

	require.NoError(t, task.deleteRedundantPools(context.Background(), lb, nil))

	require.NotNil(t, handedBack, "the pool must be handed back without the members we put in it")
	// Not just "a write happened": one built with our own members would be the opposite of the
	// point, and `"members": []` is the whole mechanism.
	sent, ok := handedBack.(*loadbalancerv2.UpdatePoolMembersRequest)
	require.True(t, ok)
	assert.Empty(t, sent.Members, "the hand-back carries the members that are not ours, which is none")
}

// The hand-back writes to a pool that is not ours, on a load balancer that is not ours, and vLB's
// answer to an empty member list is not something this code gets to assume. If that write failing
// failed the teardown, the LBC's finalizer would never come off and the Service, the Ingress and
// anything waiting on them would stay Terminating with no way out - for every user of a pinned
// load balancer, on the commonest path there is. Leaving our members behind is the lesser harm,
// and it is reported rather than swallowed quietly.
func TestDeleteStillFinishesWhenTheAdoptedPoolHandBackFails(t *testing.T) {
	const (
		lb         = "lb-user"
		theirsPool = "pool-theirs"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)
	vngcloud.EXPECT().
		ListPool(mock.Anything, lb).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: theirsPool, Name: "vks-a-b-80"}}}, nil)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lb, theirsPool).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{{Address: "10.0.0.1", ProtocolPort: 80}}}, nil)
	// what vLB might say about an empty member list - nobody has measured it yet
	vngcloud.EXPECT().
		UpdatePoolMembers(mock.Anything, lb, theirsPool, mock.Anything).
		Return(errors.New("members: must not be empty")).Once()

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      noOtherLBC(t),
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer7},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedPools: []v1alpha1.CreatedPool{{
					Id:             theirsPool,
					Name:           "vks-a-b-80",
					Adopted:        true,
					CreatedMembers: []v1alpha1.PoolMember{{IP: "10.0.0.1", Port: 80}},
				}},
			},
		},
	}

	require.NoError(t, task.deleteRedundantPools(context.Background(), lb, nil),
		"the finalizer has to come off even when a pool that is not ours cannot be tidied")
}

// noOtherLBC: this LBC is the only one in the cluster, which is the ordinary case and the one the
// hand-back is allowed to act in.
func noOtherLBC(t *testing.T) *repository.MockK8sRepository {
	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything).
		Return(nil).Maybe()
	return k8sRepo
}

// Two Ingresses of one cluster pinned to the same load balancer share a pool by design -
// validateCrossListenerDefaultPools requires the same default pool name on the same port - and the
// second LBC to meet that pool records it as adopted, because the load balancer is not its own.
// Emptying it when that LBC is torn down would take the first one's traffic with it. The members
// belong to the cluster, not to whichever LBC happens to be leaving.
func TestDeleteLeavesAnAdoptedPoolAloneWhileAnotherLBCStillUsesIt(t *testing.T) {
	const (
		lb         = "lb-user"
		sharedPool = "pool-shared"
	)

	vngcloud := repository.NewMockVngCloudRepository(t)
	vngcloud.EXPECT().
		ListPool(mock.Anything, lb).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: sharedPool, Name: "vks-a-b-80"}}}, nil)
	vngcloud.EXPECT().
		ListListenerOfLB(mock.Anything, lb).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lb, sharedPool).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{{Address: "10.0.0.1", ProtocolPort: 80}}}, nil)
	// strict mock: UpdatePoolMembers and DeletePool are undeclared, so either one fails the test

	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, list *v1alpha1.LoadBalancerConfigList, _ ...client.ListOption) error {
			list.Items = []v1alpha1.LoadBalancerConfig{{
				ObjectMeta: metav1.ObjectMeta{Name: "the-sibling", UID: "uid-sibling"},
				Status: v1alpha1.LoadBalancerConfigStatus{
					CreatedPools: []v1alpha1.CreatedPool{{Id: sharedPool, Name: "vks-a-b-80"}},
				},
			}}
			return nil
		})

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "the-one-leaving", UID: "uid-leaving"},
			Spec:       v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer7},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(lb),
				CreatedPools: []v1alpha1.CreatedPool{{
					Id:             sharedPool,
					Name:           "vks-a-b-80",
					Adopted:        true,
					CreatedMembers: []v1alpha1.PoolMember{{IP: "10.0.0.1", Port: 80}},
				}},
			},
		},
	}

	require.NoError(t, task.deleteRedundantPools(context.Background(), lb, nil))
}
