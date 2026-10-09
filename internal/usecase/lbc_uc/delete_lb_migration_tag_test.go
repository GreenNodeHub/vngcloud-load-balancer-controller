package lbc_uc

import (
	"context"
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

// targetLbId is the load balancer a migration moved to - the user's own, pinned by id.
const targetLbId = "lb-target"

// onTheTargetLB is what the target load balancer carries once this cluster has deployed onto
// it: the cluster tag listing this cluster, beside tags the controller did not author.
func onTheTargetLB() map[string]string {
	return map[string]string{
		domain.ClusterTagKey: thisClusterId,
		domain.VpcTagKey:     "net-1111",
		domain.BillingTagKey: domain.BillingTagValue,
	}
}

// The first reconcile pass of a delete that arrives mid-migration, which was already correct:
// tearing down the retiring load balancer overwrites status.createdTags on the server, but the
// object this pass works from still holds the target's record, so the id comes off.
//
// This one is the contrast, not the guard - it passes without the production change. What it
// pins is that a single pass stays correct, which is what makes the next test's failure mean
// "only once the object is read back", rather than "the target was never handled at all".
//
// The target load balancer is the user's, pinned by id, so it survives the delete - which is
// why an id left on it matters. The id is what the fleet inventory reads to tell which clusters
// use a load balancer; one naming a cluster that stopped using it makes the load balancer look
// like that cluster's forever. Deletion is decided by vng.vks.created-by-cluster, not by this
// tag.
func TestDeleteReleasesTheClusterTagOnTheTargetLoadBalancerToo(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	// The retiring load balancer's teardown, which is already known to work.
	expectRetiringTeardown(vngcloudRepo, k8sRepo)

	// The target load balancer. Nothing of this cluster's is left on it to remove - the
	// interest here is the tag alone. Its own reference lookup is a second call on top of the
	// one the retiring teardown declares.
	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Once()
	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, targetLbId).
		Return(&entityv2.LoadBalancer{UUID: targetLbId}, nil)
	vngcloudRepo.EXPECT().ListListenerOfLB(mock.Anything, targetLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloudRepo.EXPECT().ListPool(mock.Anything, targetLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)

	var written map[string]string
	vngcloudRepo.EXPECT().ListTags(mock.Anything, targetLbId).
		Return(tagList(onTheTargetLB()), nil)
	vngcloudRepo.EXPECT().InvalidateTagsCache(targetLbId).Maybe()
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, targetLbId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).Once()

	lbConfig := deleting(ptrTo(targetLbId))
	// Pinned by the user, so the controller must leave the load balancer itself in place.
	lbConfig.Spec.LoadBalancerId = ptrTo(targetLbId)
	lbConfig.Spec.Type = loadbalancerv2.LoadBalancerTypeLayer4
	// What deployTags recorded when this cluster moved onto the target.
	lbConfig.Status.CreatedTags = onTheTargetLB()

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig:     lbConfig,
	}

	require.NoError(t, task.delete(context.Background()))

	assert.NotContains(t, written, domain.ClusterTagKey,
		"nothing in this cluster points at the target load balancer any more, so its id must "+
			"not stay in the cluster tag")
	assert.Equal(t, domain.BillingTagValue, written[domain.BillingTagKey],
		"tags this cluster did not author are left exactly as they were")
}

// The same delete, one reconcile later. Tearing down the retiring load balancer ends by
// writing status.createdTags - a single field shared by every load balancer this LBC has
// touched - with the tags authored for the retiring one. The in-memory object keeps the
// target's record, because the patch helper mutates a fresh server copy, so the first pass
// gets away with it. A second pass reads the object back and finds that record gone.
//
// A delete rarely finishes in one pass: the cloud refuses writes while it is still applying
// the previous one, and any such error requeues the whole reconcile.
func TestDeleteReleasesTheClusterTagOnTheTargetAfterARequeue(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, targetLbId).
		Return(&entityv2.LoadBalancer{UUID: targetLbId}, nil)
	vngcloudRepo.EXPECT().ListListenerOfLB(mock.Anything, targetLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloudRepo.EXPECT().ListPool(mock.Anything, targetLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil)
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	var written map[string]string
	vngcloudRepo.EXPECT().ListTags(mock.Anything, targetLbId).
		Return(tagList(onTheTargetLB()), nil)
	vngcloudRepo.EXPECT().InvalidateTagsCache(targetLbId).Maybe()
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, targetLbId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).Once()

	// The object as the next pass reads it back: the retiring snapshot is gone, honoured by
	// the pass before - and so is the created-tag record it overwrote.
	lbConfig := &v1alpha1.LoadBalancerConfig{
		Spec: v1alpha1.LoadBalancerConfigSpec{
			ClusterId:      ptrTo(thisClusterId),
			Type:           loadbalancerv2.LoadBalancerTypeLayer4,
			LoadBalancerId: ptrTo(targetLbId),
		},
		Status: v1alpha1.LoadBalancerConfigStatus{
			LoadBalancerId: ptrTo(targetLbId),
			CreatedTags:    map[string]string{},
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig:     lbConfig,
	}

	require.NoError(t, task.delete(context.Background()))

	assert.NotContains(t, written, domain.ClusterTagKey,
		"the record of which tags this cluster authored is not what decides whether its own id "+
			"comes out of the cluster tag")
}

// Naming the cluster tag as ours is what asks for its removal, so it has to ask for no more
// than this cluster's own id. With another cluster still listed the key stays, carrying what
// is left of it - and that has to hold on the pass where the created-tag record is empty,
// which is the pass that reaches the new branch.
func TestDeleteLeavesAnotherClustersIdOnTheTargetAfterARequeue(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	shared := onTheTargetLB()
	shared[domain.ClusterTagKey] = otherClusterId + domain.ClusterTagValueSeparator + thisClusterId

	vngcloudRepo.EXPECT().GetLoadBalancerByID(mock.Anything, targetLbId).
		Return(&entityv2.LoadBalancer{UUID: targetLbId}, nil)
	vngcloudRepo.EXPECT().ListListenerOfLB(mock.Anything, targetLbId).
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
	vngcloudRepo.EXPECT().ListPool(mock.Anything, targetLbId).
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
	// Nobody else in *this* cluster uses it; the other cluster is a separate cluster, which
	// this lookup cannot see and which the tag is the only record of.
	k8sRepo.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil)
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	var written map[string]string
	vngcloudRepo.EXPECT().ListTags(mock.Anything, targetLbId).Return(tagList(shared), nil)
	vngcloudRepo.EXPECT().InvalidateTagsCache(targetLbId).Maybe()
	vngcloudRepo.EXPECT().CreateTags(mock.Anything, targetLbId, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).Once()

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId:      ptrTo(thisClusterId),
				Type:           loadbalancerv2.LoadBalancerTypeLayer4,
				LoadBalancerId: ptrTo(targetLbId),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptrTo(targetLbId),
				CreatedTags:    map[string]string{},
			},
		},
	}

	require.NoError(t, task.delete(context.Background()))

	assert.Equal(t, otherClusterId, written[domain.ClusterTagKey],
		"the other cluster is still using the load balancer, so the key stays and keeps its id")
}
