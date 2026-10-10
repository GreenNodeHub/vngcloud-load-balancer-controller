package lbc_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

const ownershipClusterId = "k8s-10eafaef-56e8-4dfc-878d-dd1c86fcb810"

// emptyLoadBalancer mocks the reads for a load balancer with nothing on it, so
// canDeleteWholeLoadBalancer says yes and every delete decision comes down to provenance.
// tags is what the load balancer carries, which is where provenance is read from.
func emptyLoadBalancer(vngcloud *repository.MockVngCloudRepository, tags map[string]string) {
	items := make([]*entityv2.Tag, 0, len(tags))
	for k, v := range tags {
		items = append(items, &entityv2.Tag{Key: k, Value: v})
	}
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil).Maybe()
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").
		Return(&entityv2.ListTags{Items: items}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).
		Return(nil).Maybe()
	vngcloud.EXPECT().InvalidateTagsCache("lb-1").Maybe()
}

// specLbId pins a load balancer to this LBC; createdLbId is this LBC's own record of having
// created one.
func ownershipTask(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository,
	specLbId, createdLbId *string) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				Type:           loadbalancerv2.LoadBalancerTypeLayer7,
				LoadBalancerId: specLbId,
				ClusterId:      ptr.To(ownershipClusterId),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:        ptr.To("lb-1"),
				CreatedLoadBalancerId: createdLbId,
			},
		},
	}
}

// A load balancer the user created is not ours to delete, however empty it looks: it may
// still serve workloads outside this cluster entirely. A shared ALB was destroyed this way
// once, taking its address with it and leaving every service on it dark until someone
// repointed them by hand.
//
// The mock is strict, so not expecting DeleteLoadBalancer is the assertion.
func TestDeleteLoadBalancerNeverDeletesOneTheUserCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	// Pinned by annotation, and nothing anywhere says the controller created it.
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// The ordinary case: this LBC created the load balancer, so it goes when the LBC does.
func TestDeleteLoadBalancerDeletesOneThisClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	task := ownershipTask(vngcloud, k8s, nil, ptr.To("lb-1"))

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// The case an annotation-based rule gets wrong. Several Services can share one load balancer
// by pinning its id, and the one that created it may be deleted first - after which the
// remaining LBCs look, by their Spec alone, exactly like a user who brought their own load
// balancer. The provenance tag is what tells them apart, and it says the cluster created this
// one, so the last LBC out still cleans it up.
func TestDeleteLoadBalancerDeletesAPinnedOneTheClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{domain.CreatedByClusterTagKey: ownershipClusterId})
	// Pinned by annotation, and this LBC is not the one that created it.
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// A load balancer another cluster created is not ours either, whatever our Spec looks like.
func TestDeleteLoadBalancerNeverDeletesOneAnotherClusterCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.CreatedByClusterTagKey: "k8s-4bb03c1f-7463-46c1-8bfb-ca3fc16fb085",
	})
	task := ownershipTask(vngcloud, k8s, nil, nil)

	assert.NoError(t, task.delete(context.Background()))
}

// A load balancer from before the provenance tag existed, with nothing pinning it: there is
// no evidence either way, and this is the shape of every load balancer the controller has
// ever created on its own. Deleting it is the behaviour that was there before this guard, and
// changing it would leave load balancers behind on every existing cluster.
func TestDeleteLoadBalancerDeletesAnUnpinnedOneWithNoProvenance(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	task := ownershipTask(vngcloud, k8s, nil, nil)

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// Provenance has to be recorded while the load balancer is being deployed, because by the
// time it matters the LBC that created it may be gone.
func TestDeployTagsRecordsProvenanceOnALoadBalancerThisClusterCreated(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Status.CreatedLoadBalancerId = ptr.To("lb-123")

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))
	assert.Equal(t, thisClusterId, written[domain.CreatedByClusterTagKey])
}

// Only the cluster that created a load balancer may claim it. Adopting one must not.
func TestDeployTagsDoesNotClaimALoadBalancerItAdopted(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	task.lbConfig.Spec.LoadBalancerId = ptr.To("lb-123")

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(map[string]string{}), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deployTags(context.Background(), "lb-123"))
	assert.NotContains(t, written, domain.CreatedByClusterTagKey)
}

// The provenance tag has to outlive the LBC that wrote it, so the delete path must leave it
// alone even while taking this cluster's id out of the cluster tag.
func TestDeleteRedundantTagsKeepsTheProvenanceTag(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	task := tagTask(vngcloudRepo, k8sRepo)
	expectNoOtherLBC(k8sRepo)
	task.lbConfig.Status.CreatedTags = map[string]string{
		domain.ClusterTagKey:          thisClusterId,
		domain.CreatedByClusterTagKey: thisClusterId,
		domain.VpcTagKey:              task.lbConfig.Spec.VpcId,
		domain.BillingTagKey:          domain.BillingTagValue,
	}

	onTheLoadBalancer := map[string]string{
		domain.ClusterTagKey:          thisClusterId,
		domain.CreatedByClusterTagKey: thisClusterId,
		domain.VpcTagKey:              task.lbConfig.Spec.VpcId,
		domain.BillingTagKey:          domain.BillingTagValue,
	}

	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()
	vngcloudRepo.EXPECT().InvalidateTagsCache("lb-123").Once()
	vngcloudRepo.EXPECT().ListTags(mock.Anything, "lb-123").Return(tagList(onTheLoadBalancer), nil).Once()

	var written map[string]string
	vngcloudRepo.EXPECT().
		CreateTags(mock.Anything, "lb-123", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, tags map[string]string) error {
			written = tags
			return nil
		}).
		Once()
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, task.lbConfig, mock.Anything).
		Return(nil).
		Once()

	assert.NoError(t, task.deleteRedundantTags(context.Background(), "lb-123"))
	assert.NotContains(t, written, domain.ClusterTagKey, "the departing cluster's id must go")
	assert.Equal(t, thisClusterId, written[domain.CreatedByClusterTagKey], "but provenance must stay")
}

// The hole the adoption record closes: a load balancer the user created in the portal and
// referenced by NAME. Nothing on that path sets Spec.LoadBalancerId, so before the record
// existed the "not pinned means ours" fallback claimed it - and deleted it with the Ingress.
// The same record also covers a pin that is later removed from the annotations.
func TestDeleteLoadBalancerNeverDeletesOneAdoptedByName(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, nil)
	// Unpinned - exactly what an adoption by name (or a removed pin) looks like in Spec.
	task := ownershipTask(vngcloud, k8s, nil, nil)
	task.lbConfig.Status.AdoptedLoadBalancerId = ptr.To("lb-1")

	// strict mock: DeleteLoadBalancer undeclared, so any call fails the test
	assert.NoError(t, task.delete(context.Background()))
}

// Adoption is recorded the moment the by-name lookup finds an existing load balancer,
// before anything else can fail - otherwise a crash right after adoption would leave the
// load balancer unprotected.
func TestDeployLoadBalancerRecordsAdoptionByName(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)

	vngcloud.EXPECT().GetLoadBalancerByName(mock.Anything, "user-made-lb").
		Return(&entityv2.LoadBalancer{UUID: "lb-1", Name: "user-made-lb"}, nil).Once()
	k8s.EXPECT().PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	// stop the walk right after the adoption record is written (ensureExist was handed the
	// entity, so its first cloud call is the active-wait)
	vngcloud.EXPECT().WaitForLBActive(mock.Anything, "lb-1").
		Return(nil, errors.New("stop here: adoption must already be recorded")).Once()

	task := ownershipTask(vngcloud, k8s, nil, nil)
	task.lbConfig.Status.LoadBalancerId = nil
	task.lbConfig.Spec.LoadBalancerName = "user-made-lb"

	_, err := task.deployLoadBalancer(context.Background(), nil)
	assert.Error(t, err)

	if assert.NotNil(t, task.lbConfig.Status.AdoptedLoadBalancerId) {
		assert.Equal(t, "lb-1", *task.lbConfig.Status.AdoptedLoadBalancerId)
	}
}

const siblingClusterId = "k8s-4bb03c1f-7463-46c1-8bfb-ca3fc16fb085"

// soleLBCInTheCluster: carrying a cluster tag sends the teardown down the withdraw-my-id path,
// which asks whether a sibling LBC in this cluster still points at the load balancer (none does)
// and then records the tags it wrote.
func soleLBCInTheCluster(k8s *repository.MockK8sRepository) {
	k8s.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Maybe()
	k8s.EXPECT().PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
}

// #33310 - a load balancer one VKS cluster created and another pinned outlived both of them.
// Every step was right on its own: the creator could not delete it while the other cluster was
// still using it, and the other cluster could not delete something it had not created. Nobody was
// left holding the job, and the load balancer went on being billed.
//
// What breaks the deadlock is that the provenance tag names a cluster at all: a load balancer with
// one was made by VKS, not by the user, so the last cluster to stop using it may clear up. The
// cluster list is how "last" is known - after this cluster takes itself out, nobody else is named.
func TestDeleteLoadBalancerLetsTheLastClusterOutDeleteOneASiblingCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.CreatedByClusterTagKey: siblingClusterId, // the cluster that made it, already gone
		domain.ClusterTagKey:          ownershipClusterId,
	})
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// And not a moment earlier. While another cluster is still named on it, leaving is leaving - the
// load balancer is still somebody's.
func TestDeleteLoadBalancerLeavesOneASiblingCreatedWhileAnotherClusterStillUsesIt(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.CreatedByClusterTagKey: siblingClusterId,
		domain.ClusterTagKey:          ownershipClusterId + domain.ClusterTagValueSeparator + siblingClusterId,
	})
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)
	// strict mock: DeleteLoadBalancer is undeclared, so deleting it fails the test outright

	assert.NoError(t, task.delete(context.Background()))
}

// The guard that matters most: a load balancer of the user's carries no provenance tag, so being
// the last cluster to leave it confers nothing. This is the case the whole ownership rule exists
// for, and the new one must not reach past it.
func TestDeleteLoadBalancerStillNeverDeletesTheUsersEvenWhenLastOut(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.ClusterTagKey: ownershipClusterId, // only us on it, and no provenance tag at all
	})
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// "Last" is a claim about a record this cluster is part of. A cluster list that never named us is
// not describing us, and reading a deletion out of it would be guesswork about a tag we cannot
// account for.
func TestDeleteLoadBalancerDoesNotClaimToBeLastOffAListItIsNotOn(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud, map[string]string{
		domain.CreatedByClusterTagKey: siblingClusterId,
		// and no cluster list at all: nothing here says we were ever a user of it
	})
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// The shape the ticket actually describes, rather than a bare load balancer: the sibling that
// created it is gone and this cluster is serving from the listener it left behind, adopted by
// port. Adoption is why the old rule could never finish - what this cluster adopted is not what
// this cluster created, so the load balancer never looked empty and never looked ours.
func TestDeleteLoadBalancerLastClusterOutTakesWhatItAdoptedFromTheSibling(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{{UUID: "lis-1", Name: "sibling-listener"}}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: "pool-1", Name: "vks-pool"}}}, nil).Maybe()
	vngcloud.EXPECT().GetPoolMembers(mock.Anything, "lb-1", "pool-1").
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil).Maybe()
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").Return(&entityv2.ListTags{Items: []*entityv2.Tag{
		{Key: domain.CreatedByClusterTagKey, Value: siblingClusterId},
		{Key: domain.ClusterTagKey, Value: ownershipClusterId},
	}}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).Return(nil).Maybe()
	vngcloud.EXPECT().InvalidateTagsCache("lb-1").Maybe()
	soleLBCInTheCluster(k8s)

	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)
	task.lbConfig.Spec.Type = loadbalancerv2.LoadBalancerTypeLayer4
	task.lbConfig.Status.CreatedListeners = []v1alpha1.CreatedListener{{Id: "lis-1", Port: 80, Adopted: true}}
	task.lbConfig.Status.CreatedPools = []v1alpha1.CreatedPool{{Id: "pool-1", Name: "vks-pool"}}

	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").Return(nil).Once()

	assert.NoError(t, task.delete(context.Background()))
}

// vksLoadBalancerSharedWith mocks an empty load balancer a sibling cluster created. Each entry in
// users is the cluster list one read of the tags hands back, in order, so a test can show the list
// changing under the teardown; the last entry answers any further reads.
func vksLoadBalancerSharedWith(vngcloud *repository.MockVngCloudRepository, users ...string) {
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).Return(nil).Maybe()
	vngcloud.EXPECT().InvalidateTagsCache("lb-1").Maybe()
	tags := func(list string) *entityv2.ListTags {
		return &entityv2.ListTags{Items: []*entityv2.Tag{
			{Key: domain.CreatedByClusterTagKey, Value: siblingClusterId},
			{Key: domain.ClusterTagKey, Value: list},
		}}
	}
	for _, list := range users {
		vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").Return(tags(list), nil).Once()
	}
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").Return(tags(users[len(users)-1]), nil).Maybe()
}

// The cluster tag names clusters, not LBCs, so a cluster id on it can stand for a sibling LBC in
// this same cluster - which is why the tag teardown asks who else points at the load balancer
// before dropping the id. Being the only cluster on the tag therefore does not make this LBC the
// last user: the sibling is behind that same id, and deleting the load balancer takes it out from
// under them.
func TestDeleteLoadBalancerIsNotLastOutWhileASiblingLBCInThisClusterUsesIt(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	vksLoadBalancerSharedWith(vngcloud, ownershipClusterId)
	expectOtherLBC(k8s, "lb-1")
	k8s.EXPECT().ListLoadBalancerConfig(mock.Anything, mock.Anything).Return(nil).Maybe()
	k8s.EXPECT().PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// Tags are served from a five-minute cache, and the repository's contract is that a cached read
// may only ever say "no write needed" - never decide a write. Deleting a load balancer is the
// least reversible write there is, so the cluster list it rests on has to be read through.
func TestDeleteLoadBalancerRereadsTagsBeforeDecidingItIsLastOut(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	// first read is the cached one and says we are alone; the read-through finds the cluster
	// that added itself while that entry was being served
	vksLoadBalancerSharedWith(vngcloud, ownershipClusterId,
		ownershipClusterId+domain.ClusterTagValueSeparator+siblingClusterId)
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)

	assert.NoError(t, task.delete(context.Background()))
}

// An LBC with no usable cluster id cannot be anybody's last user. Empty splits into one empty
// field and subtracts to nothing, so without this both halves of the test pass vacuously.
func TestDeleteLoadBalancerIsNotLastOutWithoutAUsableClusterId(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	vksLoadBalancerSharedWith(vngcloud, "")
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)
	task.lbConfig.Spec.ClusterId = ptr.To("")

	assert.NoError(t, task.delete(context.Background()))
}

// The late delete: the load balancer did not look coverable at first, so the teardown went the
// long way round - clearing listeners and pools, each of which can wait minutes on the load
// balancer going ACTIVE - and only then found it empty. The cluster list read before all that is
// no longer evidence. A cluster adopting this load balancer writes its id into the list before it
// creates anything on it, so "empty" does not mean "unclaimed", and the claim has to be checked
// again against a fresh list immediately before the delete.
func TestDeleteLoadBalancerConfirmsLastOutAgainBeforeTheLateDelete(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	// first read is canDeleteWholeLoadBalancer: a listener this LBC has no record of, so the
	// whole-load-balancer delete is off; by the time isLoadBalancerEmpty looks, it is gone
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{{UUID: "lis-9", Name: "someone-elses"}}}, nil).Once()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).Return(nil).Maybe()
	vngcloud.EXPECT().InvalidateTagsCache("lb-1").Maybe()
	tags := func(list string) *entityv2.ListTags {
		return &entityv2.ListTags{Items: []*entityv2.Tag{
			{Key: domain.CreatedByClusterTagKey, Value: siblingClusterId},
			{Key: domain.ClusterTagKey, Value: list},
		}}
	}
	// alone on the list when the teardown starts, and alone on the read-through that follows
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").Return(tags(ownershipClusterId), nil).Twice()
	// by the time the long way round is done, a cluster has adopted it
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").
		Return(tags(ownershipClusterId+domain.ClusterTagValueSeparator+siblingClusterId), nil).Maybe()
	soleLBCInTheCluster(k8s)
	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1"), nil)
	task.lbConfig.Spec.Type = loadbalancerv2.LoadBalancerTypeLayer4

	assert.NoError(t, task.delete(context.Background()))
}
