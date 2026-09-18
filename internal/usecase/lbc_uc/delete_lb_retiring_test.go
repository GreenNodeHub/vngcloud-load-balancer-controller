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
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// QC-2c: the LBC is deleted while a migration is still in flight. The snapshot in status is the
// only remaining record of what this cluster put on the load balancer it was migrating away
// from, so the delete path has to act on it before anything else - otherwise the listeners,
// pools and the cluster tag on the old load balancer are stranded with nothing left pointing
// at them. The deploy path's deferred teardown never gets another turn: the object is going away.

// deleting is an LBC on its way out, still carrying the retiring snapshot.
func deleting(currentLbId *string) *v1alpha1.LoadBalancerConfig {
	return &v1alpha1.LoadBalancerConfig{
		Spec: v1alpha1.LoadBalancerConfigSpec{
			ClusterId: ptrTo(thisClusterId),
			Type:      loadbalancerv2.LoadBalancerTypeLayer4,
		},
		Status: v1alpha1.LoadBalancerConfigStatus{
			LoadBalancerId:       currentLbId,
			RetiringLoadBalancer: retiringSnapshot(),
		},
	}
}

// expectRetiringTeardown declares every cloud call the teardown of the snapshot makes, and
// nothing else - so a delete path that skipped it, or that reached the current load balancer
// first, fails on an undeclared call rather than on a soft assertion.
func expectRetiringTeardown(vngcloudRepo *repository.MockVngCloudRepository, k8sRepo *repository.MockK8sRepository) {
	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)
	vngcloudRepo.EXPECT().ListListenerOfLB(mock.Anything, oldLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{{UUID: "listener-1", ProtocolPort: 80}}}, nil)
	vngcloudRepo.EXPECT().DeleteListener(mock.Anything, oldLbId, "listener-1").Return(nil).Once()
	vngcloudRepo.EXPECT().ListPool(mock.Anything, oldLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: "pool-1", Name: "vks-a-b-80"}}}, nil)
	vngcloudRepo.EXPECT().GetPoolMembers(mock.Anything, oldLbId, "pool-1").
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil)
	vngcloudRepo.EXPECT().DeletePool(mock.Anything, oldLbId, "pool-1").Return(nil).Once()
	vngcloudRepo.EXPECT().WaitForLBActive(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)

	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Once()
	onTheOldLB := retiringSnapshot().CreatedTags
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLB), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache(oldLbId).Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLB), nil).Once()
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, oldLbId, mock.Anything).Return(nil).Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
}

// The leak this closes: migration parks the snapshot before it has a new load balancer to
// record, so status can hold a retiring load balancer and no current one. An early return on
// the empty current id would walk straight past the only thing left to clean up.
func TestDeleteSweepsTheRetiringLoadBalancerEvenWithNoCurrentOne(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	expectRetiringTeardown(vngcloudRepo, k8sRepo)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig:     deleting(nil),
	}

	require.NoError(t, task.delete(context.Background()))
	assert.Nil(t, task.lbConfig.Status.RetiringLoadBalancer,
		"the snapshot is cleared once it has been acted on, so a retry cannot tear it down twice")
}

// With a current load balancer as well, the retiring one still has to be dealt with first: the
// current load balancer's own delete can return early or fail, and either way the snapshot must
// already have been honoured. Failing the teardown proves the ordering - the current load
// balancer is never inspected, which the strict mock enforces by having no expectation for it.
func TestDeleteTearsDownTheRetiringLoadBalancerBeforeTheCurrentOne(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)

	// Not the "cannot get load balancer" shape, so it is a real failure rather than
	// "already gone" - the teardown propagates it.
	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(nil, errors.New("vserver returned 503"))

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      repository.NewMockK8sRepository(t),
		lbConfig:     deleting(ptrTo("lb-current")),
	}

	err := task.delete(context.Background())

	require.Error(t, err, "a teardown that failed must requeue, not be swallowed by carrying on")
	assert.NotNil(t, task.lbConfig.Status.RetiringLoadBalancer,
		"the snapshot survives a failed teardown, so the next pass still knows what to clean up")
}

// The cluster tag on the old load balancer is what provenance is read from later. Leaving this
// cluster's id there after the object is gone makes the load balancer look like this cluster's
// forever - which is what decides, in item 1, whether it is ever deleted.
func TestDeleteReleasesTheClusterTagOnTheRetiringLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)
	vngcloudRepo.EXPECT().ListListenerOfLB(mock.Anything, oldLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{{UUID: "listener-1", ProtocolPort: 80}}}, nil)
	vngcloudRepo.EXPECT().DeleteListener(mock.Anything, oldLbId, "listener-1").Return(nil).Once()
	vngcloudRepo.EXPECT().ListPool(mock.Anything, oldLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: "pool-1", Name: "vks-a-b-80"}}}, nil)
	vngcloudRepo.EXPECT().GetPoolMembers(mock.Anything, oldLbId, "pool-1").
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil)
	vngcloudRepo.EXPECT().DeletePool(mock.Anything, oldLbId, "pool-1").Return(nil).Once()
	vngcloudRepo.EXPECT().WaitForLBActive(mock.Anything, oldLbId).
		Return(&entityv2.LoadBalancer{UUID: oldLbId}, nil)

	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Once()
	onTheOldLB := retiringSnapshot().CreatedTags
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLB), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache(oldLbId).Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, oldLbId).Return(tagList(onTheOldLB), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, oldLbId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig:     deleting(nil),
	}

	require.NoError(t, task.delete(context.Background()))

	assert.NotContains(t, written, domain.ClusterTagKey,
		"this cluster is gone from the old load balancer, so its id must not stay in the cluster tag")
	assert.Equal(t, domain.BillingTagValue, written[domain.BillingTagKey],
		"tags this cluster did not author are left exactly as they were")
}
