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

const oldLbId = "lb-old"

// retiringSnapshot is what a migration parks in status: the old load balancer and the
// record of what this LBC created on it.
func retiringSnapshot() *v1alpha1.RetiringLoadBalancer {
	return &v1alpha1.RetiringLoadBalancer{
		Id:               oldLbId,
		CreatedListeners: []v1alpha1.CreatedListener{{Id: "listener-1", Port: 80}},
		CreatedPools:     []v1alpha1.CreatedPool{{Id: "pool-1", Name: "vks-a-b-80"}},
		CreatedTags: map[string]string{
			domain.ClusterTagKey: thisClusterId,
			domain.VpcTagKey:     "net-1111",
			domain.BillingTagKey: domain.BillingTagValue,
		},
	}
}

// Migrating away parks the old load balancer for a deferred teardown instead of stripping
// it on the spot: at that moment the new load balancer carries nothing, so tearing the old
// one down first would leave a window where neither side serves. The strict mock proves no
// cloud write of any kind happens during the migration itself - the old load balancer keeps
// serving untouched.
func TestMigrateDoesNotTouchTheOldLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	// the only cloud interaction allowed: ensureExistLoadBalancer inspecting the NEW one
	vngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, "lb-new").
		Return(nil, errors.New("stop the walk here: the point is proven before the new LB is inspected")).Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId:      ptrTo(thisClusterId),
				LoadBalancerId: ptrTo("lb-new"),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:   ptrTo(oldLbId),
				CreatedListeners: []v1alpha1.CreatedListener{{Id: "listener-1", Port: 80}},
				CreatedPools:     []v1alpha1.CreatedPool{{Id: "pool-1", Name: "vks-a-b-80"}},
				CreatedTags:      map[string]string{domain.ClusterTagKey: thisClusterId},
			},
		},
	}

	_, err := task.migrateLoadBalancer(context.Background(), oldLbId, "lb-new")
	require.Error(t, err) // the sentinel from the stubbed GetLoadBalancerByID

	// the snapshot carries everything the deferred teardown will need
	retiring := task.lbConfig.Status.RetiringLoadBalancer
	require.NotNil(t, retiring, "the old load balancer must be parked for deferred teardown")
	assert.Equal(t, oldLbId, retiring.Id)
	assert.Len(t, retiring.CreatedListeners, 1)
	assert.Len(t, retiring.CreatedPools, 1)
	assert.Equal(t, thisClusterId, retiring.CreatedTags[domain.ClusterTagKey])

	// the live record now belongs to the new load balancer
	assert.Empty(t, task.lbConfig.Status.CreatedListeners)
	assert.Empty(t, task.lbConfig.Status.CreatedPools)
	assert.Empty(t, task.lbConfig.Status.CreatedTags,
		"stale createdTags carried into the new load balancer would be treated as ours to drop there")

	// and the new load balancer is on record as adopted, so it is never ours to delete
	require.NotNil(t, task.lbConfig.Status.AdoptedLoadBalancerId)
	assert.Equal(t, "lb-new", *task.lbConfig.Status.AdoptedLoadBalancerId)
}

// The deferred teardown works entirely from the snapshot - by the time it runs, live status
// describes the new load balancer. It removes what the snapshot records and never deletes
// the load balancer itself (strict mock: DeleteLoadBalancer is undeclared).
func TestTeardownRetiringWorksFromTheSnapshotNotLiveStatus(t *testing.T) {
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

	// tag release: nobody else uses the old LB, so the cluster id comes off
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
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId: ptrTo(thisClusterId),
				Type:      loadbalancerv2.LoadBalancerTypeLayer4,
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				// live status already describes the NEW load balancer; the teardown must not read it
				LoadBalancerId: ptrTo("lb-new"),
				CreatedPools:   []v1alpha1.CreatedPool{{Id: "pool-new", Name: "vks-new"}},
			},
		},
	}

	require.NoError(t, task.teardownRetiringLoadBalancer(context.Background(), retiringSnapshot()))
	assert.NotContains(t, written, domain.ClusterTagKey,
		"this cluster no longer uses the old load balancer, so its id must come off")
}

// A load balancer that is already gone is not an error: there is nothing left to
// reclaim, and failing here would block the reconcile forever.
func TestTeardownRetiringIgnoresMissingLoadBalancer(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	vngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, oldLbId).
		Return(nil, errors.New("Cannot get load balancer with id "+oldLbId))

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	assert.NoError(t, task.teardownRetiringLoadBalancer(context.Background(), retiringSnapshot()))
}

func ptrTo[T any](v T) *T { return &v }
