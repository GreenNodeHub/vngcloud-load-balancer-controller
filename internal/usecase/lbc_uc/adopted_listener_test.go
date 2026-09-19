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
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// Every load balancer created on the portal arrives with a listener on port 80 and a default
// pool. deployListener matches listeners by port alone, so that listener is what a pinned
// Ingress lands on - and because an adopted listener used to be recorded in
// status.createdListeners exactly like one the controller created, the teardown deleted it.
//
// Measured on a pristine ALB before this change: the controller created 0 listeners on it and
// deleted 1. The load balancer survived, which item 1 guarantees, but came back without its
// listener and with its default pool orphaned.
const (
	usersListenerId = "lis-08d43abf-9a85-3ea9-91f2-08945d7a33e6"
	usersPoolId     = "pool-97852dbd-23d1-3113-9d89-80c52562ec18"
	ourPolicyId     = "pol-ours"
)

// adoptedListenerTask is an LBC on its way out, whose status records the user's port-80 listener
// as adopted, with one policy this LBC put on it.
func adoptedListenerTask(
	vngcloudRepo *repository.MockVngCloudRepository,
	k8sRepo *repository.MockK8sRepository,
	adopted bool,
) *defaultModelDeployTask {
	listener := v1alpha1.CreatedListener{
		Id:              usersListenerId,
		Port:            80,
		CreatedPolicies: []v1alpha1.CreatedPolicy{{Id: ourPolicyId}},
		Adopted:         adopted,
	}
	if adopted {
		listener.OriginalDefaultPoolId = ptrTo(usersPoolId)
	}

	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId: ptrTo(thisClusterId),
				Type:      loadbalancerv2.LoadBalancerTypeLayer7,
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:   ptrTo("lb-user"),
				CreatedListeners: []v1alpha1.CreatedListener{listener},
			},
		},
	}
}

// onTheLoadBalancer declares the cloud side: the user's listener, its default pool already
// stripped by an earlier reconcile, carrying the one policy this LBC created.
func onTheLoadBalancer(vngcloudRepo *repository.MockVngCloudRepository) {
	vngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-user").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{
			{UUID: usersListenerId, ProtocolPort: 80, DefaultPoolId: ""},
		}}, nil)
	vngcloudRepo.EXPECT().
		ListPolicyOfListener(mock.Anything, "lb-user", usersListenerId).
		Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{{UUID: ourPolicyId}}}, nil)
	vngcloudRepo.EXPECT().
		DeletePolicy(mock.Anything, "lb-user", usersListenerId, ourPolicyId).
		Return(nil).Once()
	vngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-user").
		Return(&entityv2.LoadBalancer{UUID: "lb-user"}, nil)
}

// The break this catches: the teardown deleting a listener that was on the load balancer before
// this cluster ever saw it. The mock is strict and DeleteListener is undeclared, so a delete
// fails the test outright rather than through an assertion.
func TestDeleteRedundantListenersNeverDeletesAnAdoptedListener(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	onTheLoadBalancer(vngcloudRepo)

	// What the listener must be left with: the default pool it had before we touched it.
	var restoredTo *string
	vngcloudRepo.EXPECT().
		UpdateListener(mock.Anything, "lb-user", usersListenerId, mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, opt loadbalancerv2.IUpdateListenerRequest) error {
			if req, ok := opt.(*loadbalancerv2.UpdateListenerRequest); ok {
				restoredTo = ptrTo(req.DefaultPoolId)
			}
			return nil
		}).Once()

	task := adoptedListenerTask(vngcloudRepo, k8sRepo, true)

	// The delete path: nothing is wanted any more.
	err := task.deleteRedundantListenersFrom(context.Background(), "lb-user",
		task.lbConfig.Status.CreatedListeners, []v1alpha1.CreatedListener{}, []v1alpha1.CreatedPool{})

	require.NoError(t, err)
	require.NotNil(t, restoredTo, "the adopted listener must be updated to put its default pool back")
	assert.Equal(t, usersPoolId, *restoredTo,
		"leaving the listener in place but with an empty default pool still hands back a broken load balancer")
}

// The other half of the contract: a listener this LBC really did create is still swept, or every
// teardown would leak one. Adoption must narrow the delete, not disable it.
func TestDeleteRedundantListenersStillDeletesOneThisLBCCreated(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	vngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-user").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{
			{UUID: usersListenerId, ProtocolPort: 80, DefaultPoolId: ""},
		}}, nil)
	vngcloudRepo.EXPECT().
		ListPolicyOfListener(mock.Anything, "lb-user", usersListenerId).
		Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{{UUID: ourPolicyId}}}, nil)
	vngcloudRepo.EXPECT().
		DeleteListener(mock.Anything, "lb-user", usersListenerId).
		Return(nil).Once()
	vngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-user").
		Return(&entityv2.LoadBalancer{UUID: "lb-user"}, nil)

	task := adoptedListenerTask(vngcloudRepo, k8sRepo, false)

	require.NoError(t, task.deleteRedundantListenersFrom(context.Background(), "lb-user",
		task.lbConfig.Status.CreatedListeners, []v1alpha1.CreatedListener{}, []v1alpha1.CreatedPool{}))
}

// ---------------------------------------------------------------------------
// Recording the adoption
// ---------------------------------------------------------------------------

// applyStatusPatch makes the mock run the mutation against the object, so these tests can assert
// on the status that results rather than on the fact that a patch was requested.
func applyStatusPatch(k8sRepo *repository.MockK8sRepository) {
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, lbc *v1alpha1.LoadBalancerConfig,
			mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			mutate(ctx, lbc)
			return nil
		})
}

// taskWithListeners models an LBC on a load balancer this cluster ADOPTED - the user's ALB. That
// is the only situation in which a listener can belong to someone else, so it is the setup every
// adoption test needs; on a load balancer of our own making adoption is impossible by definition.
func taskWithListeners(k8sRepo *repository.MockK8sRepository, listeners ...v1alpha1.CreatedListener) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:  logrus.NewEntry(logrus.New()),
		k8sRepo: k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:        ptrTo("lb-user"),
				AdoptedLoadBalancerId: ptrTo("lb-user"),
				CreatedListeners:      listeners,
			},
		},
	}
}

// The value that has to survive: what the listener's default pool was before this LBC stripped
// it. The second reconcile sees the stripped listener, so recording the original again would
// overwrite it with "" - and the restore would then hand back the same broken listener.
func TestAdoptingAListenerRecordsTheOriginalDefaultPoolOnlyOnce(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	applyStatusPatch(k8sRepo)
	task := taskWithListeners(k8sRepo)

	// first reconcile: the listener still has the user's default pool
	require.NoError(t, task.statusAdoptListener(context.Background(), usersListenerId, 80, usersPoolId))
	// second reconcile: we have since stripped it, and see ""
	require.NoError(t, task.statusAdoptListener(context.Background(), usersListenerId, 80, ""))

	require.Len(t, task.lbConfig.Status.CreatedListeners, 1)
	rec := task.lbConfig.Status.CreatedListeners[0]
	assert.True(t, rec.Adopted)
	require.NotNil(t, rec.OriginalDefaultPoolId)
	assert.Equal(t, usersPoolId, *rec.OriginalDefaultPoolId,
		"the original default pool is recorded at adoption and never overwritten afterwards")
}

// The trap on the other side: a listener this LBC created is found by port on the next reconcile
// and reaches the same code path. Marking it adopted there would stop the teardown ever deleting
// it, turning a fixed bug into a leak. Already being in status is what says it is ours.
func TestAListenerThisLBCCreatedIsNotMarkedAdoptedOnTheNextPass(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	applyStatusPatch(k8sRepo)
	task := taskWithListeners(k8sRepo, v1alpha1.CreatedListener{Id: "lis-ours", Port: 80})

	require.NoError(t, task.statusAdoptListener(context.Background(), "lis-ours", 80, "pool-ours"))

	require.Len(t, task.lbConfig.Status.CreatedListeners, 1)
	rec := task.lbConfig.Status.CreatedListeners[0]
	assert.False(t, rec.Adopted, "a listener already on our books was created by us, not adopted")
	assert.Nil(t, rec.OriginalDefaultPoolId)
}

// An unnameable listener must not be recorded, for the same reason statusAddListener refuses one:
// id is the key of a map-list, so an empty one has the API server reject every status patch.
func TestAdoptingAListenerRefusesAnEmptyId(t *testing.T) {
	task := taskWithListeners(repository.NewMockK8sRepository(t))

	err := task.statusAdoptListener(context.Background(), "", 80, usersPoolId)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "need to retry")
}

// ---------------------------------------------------------------------------
// The adoption has to survive the end of the reconcile
// ---------------------------------------------------------------------------

// deploy() finishes by overwriting status.createdListeners wholesale with what deployListeners
// returned. So it is not enough for statusAdoptListener to write the adoption: the value
// deployListener hands back has to carry it too, or the record lives only between those two
// writes and the teardown - a later reconcile - reads a listener with Adopted false and deletes
// the user's listener exactly as before.
//
// Measured on a pristine ALB with the first version of this fix: status showed adopted=true
// mid-reconcile, and the listener was deleted anyway.
func TestDeployListenerCarriesTheAdoptionIntoWhatStatusIsOverwrittenWith(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	applyStatusPatch(k8sRepo)

	// Stripping the user's default pool is an update, which is what displaces it in the first place.
	vngcloudRepo.EXPECT().
		UpdateListener(mock.Anything, "lb-user", usersListenerId, mock.Anything).
		Return(nil).Once()
	vngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-user").
		Return(&entityv2.LoadBalancer{UUID: "lb-user"}, nil)

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId:      ptrTo(thisClusterId),
				Type:           loadbalancerv2.LoadBalancerTypeLayer4,
				LoadBalancerId: ptrTo("lb-user"),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:        ptrTo("lb-user"),
				AdoptedLoadBalancerId: ptrTo("lb-user"),
			},
		},
	}

	// The load balancer already has a listener on this port, carrying the user's default pool.
	onLB := &entityv2.ListListeners{Items: []*entityv2.Listener{{
		UUID:          usersListenerId,
		ProtocolPort:  80,
		Protocol:      string(loadbalancerv2.ListenerProtocolTCP),
		DefaultPoolId: usersPoolId,
	}}}

	got, err := task.deployListener(context.Background(), "lb-user",
		v1alpha1.Listener{Protocol: loadbalancerv2.ListenerProtocolTCP, ProtocolPort: 80},
		onLB, []v1alpha1.CreatedPool{}, []v1alpha1.CreatedCertificate{})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Adopted,
		"the value deploy() writes over status with must say the listener was adopted")
	require.NotNil(t, got.OriginalDefaultPoolId,
		"and must carry the default pool it displaced, or the teardown has nothing to restore")
	assert.Equal(t, usersPoolId, *got.OriginalDefaultPoolId)
}

// On the second reconcile the listener is found by port again, but by then it carries this LBC's
// pools - so the original must come from the record already in status, never from what the
// listener looks like now.
func TestDeployListenerKeepsTheOriginalDefaultPoolFromStatusOnLaterPasses(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)
	applyStatusPatch(k8sRepo)
	vngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-user").
		Return(&entityv2.LoadBalancer{UUID: "lb-user"}, nil).Maybe()

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
				LoadBalancerId:        ptrTo("lb-user"),
				AdoptedLoadBalancerId: ptrTo("lb-user"),
				CreatedListeners: []v1alpha1.CreatedListener{{
					Id:                    usersListenerId,
					Port:                  80,
					Adopted:               true,
					OriginalDefaultPoolId: ptrTo(usersPoolId),
				}},
			},
		},
	}

	// The listener as it stands now: our pool, not the user's.
	onLB := &entityv2.ListListeners{Items: []*entityv2.Listener{{
		UUID:          usersListenerId,
		ProtocolPort:  80,
		Protocol:      string(loadbalancerv2.ListenerProtocolTCP),
		DefaultPoolId: "",
	}}}

	got, err := task.deployListener(context.Background(), "lb-user",
		v1alpha1.Listener{Protocol: loadbalancerv2.ListenerProtocolTCP, ProtocolPort: 80},
		onLB, []v1alpha1.CreatedPool{}, []v1alpha1.CreatedCertificate{})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Adopted)
	require.NotNil(t, got.OriginalDefaultPoolId)
	assert.Equal(t, usersPoolId, *got.OriginalDefaultPoolId,
		"the original is whatever was recorded at adoption, not what the listener carries now")
}

// applyStatusPatchToFresh models what the patch helper really does: it mutates a FRESH copy read
// from the API server, not the object it was handed. The two disagree whenever the in-memory copy
// predates a write - which is the normal state of affairs partway through a reconcile.
func applyStatusPatchToFresh(k8sRepo *repository.MockK8sRepository, fresh *v1alpha1.LoadBalancerConfig) {
	k8sRepo.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ *v1alpha1.LoadBalancerConfig,
			mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			mutate(ctx, fresh)
			return nil
		})
}

// The break this catches, measured in the core envtest suite: a listener the controller had
// created was marked adopted and then left in place, so changing a Service's port left the old
// listener behind and the suite failed on finding two.
//
// It happens when the in-memory copy is stale. The mutation runs against the fresh object, sees
// the listener already recorded, and correctly changes nothing - but the in-memory copy still
// lacks it, so deciding again from that copy reads "not on our books yet" and adopts a listener
// that is ours. The decision has to be made once, on the fresh object, and carried out.
func TestAdoptingNeverMarksAListenerTheControllerCreatedWhenTheInMemoryCopyIsStale(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)

	// what the API server holds: the listener this LBC created, two reconciles ago
	fresh := &v1alpha1.LoadBalancerConfig{
		Status: v1alpha1.LoadBalancerConfigStatus{
			CreatedListeners: []v1alpha1.CreatedListener{{Id: "lis-ours", Port: 80}},
		},
	}
	applyStatusPatchToFresh(k8sRepo, fresh)

	// what this reconcile is holding: nothing yet
	task := taskWithListeners(k8sRepo)

	require.NoError(t, task.statusAdoptListener(context.Background(), "lis-ours", 80, "pool-ours"))

	require.Len(t, fresh.Status.CreatedListeners, 1, "the fresh object must not gain a duplicate")
	assert.False(t, fresh.Status.CreatedListeners[0].Adopted,
		"a listener already on our books was created by us")

	require.Len(t, task.lbConfig.Status.CreatedListeners, 1)
	assert.False(t, task.lbConfig.Status.CreatedListeners[0].Adopted,
		"and the in-memory copy must agree, or deployListener carries a false adoption into status")
	assert.Nil(t, task.lbConfig.Status.CreatedListeners[0].OriginalDefaultPoolId)
}

// The same seam the other way: a listener that really is new to us is adopted, and the in-memory
// copy learns it, so deployListener can carry it into the value deploy() overwrites status with.
func TestAdoptingRecordsANewListenerOnBothCopies(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	fresh := &v1alpha1.LoadBalancerConfig{}
	applyStatusPatchToFresh(k8sRepo, fresh)
	task := taskWithListeners(k8sRepo)

	require.NoError(t, task.statusAdoptListener(context.Background(), usersListenerId, 80, usersPoolId))

	for name, ls := range map[string][]v1alpha1.CreatedListener{
		"fresh":     fresh.Status.CreatedListeners,
		"in-memory": task.lbConfig.Status.CreatedListeners,
	} {
		require.Len(t, ls, 1, name)
		assert.True(t, ls[0].Adopted, name)
		require.NotNil(t, ls[0].OriginalDefaultPoolId, name)
		assert.Equal(t, usersPoolId, *ls[0].OriginalDefaultPoolId, name)
	}
}

// Absence from status is not evidence of foreign ownership. status.createdListeners is rewritten
// wholesale at the end of every deploy, so a pass that resolves no listeners empties it - and the
// next pass then finds this LBC's own listener on the load balancer with nothing on the books.
//
// Measured in the core envtest suite: freshHas=0, and a listener the controller had created was
// adopted and left behind, so changing a Service's port left two listeners on the load balancer.
//
// What is actually reliable is the load balancer's own provenance. Nothing on a load balancer this
// cluster created can belong to anyone else, so adoption is impossible there whatever status says.
func TestAdoptionIsImpossibleOnALoadBalancerThisClusterCreated(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	fresh := &v1alpha1.LoadBalancerConfig{} // status wiped: no listeners recorded
	applyStatusPatchToFresh(k8sRepo, fresh)

	task := taskWithListeners(k8sRepo)
	task.lbConfig.Status.LoadBalancerId = ptrTo("lb-ours")
	task.lbConfig.Status.AdoptedLoadBalancerId = nil
	task.lbConfig.Status.CreatedLoadBalancerId = ptrTo("lb-ours")

	require.NoError(t, task.statusAdoptListener(context.Background(), "lis-ours", 80, "pool-ours"))

	require.Len(t, task.lbConfig.Status.CreatedListeners, 1)
	assert.False(t, task.lbConfig.Status.CreatedListeners[0].Adopted,
		"this cluster created the load balancer, so the listener on it cannot be someone else's")
	assert.Nil(t, task.lbConfig.Status.CreatedListeners[0].OriginalDefaultPoolId)
}

// And the case the whole change exists for: on a load balancer this LBC adopted, a listener it has
// never recorded is the user's - that is the port-80 listener every portal-created ALB ships with.
func TestAdoptionStillHappensOnALoadBalancerThisClusterAdopted(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	fresh := &v1alpha1.LoadBalancerConfig{}
	applyStatusPatchToFresh(k8sRepo, fresh)

	task := taskWithListeners(k8sRepo)

	require.NoError(t, task.statusAdoptListener(context.Background(), usersListenerId, 80, usersPoolId))

	require.Len(t, task.lbConfig.Status.CreatedListeners, 1)
	assert.True(t, task.lbConfig.Status.CreatedListeners[0].Adopted)
	require.NotNil(t, task.lbConfig.Status.CreatedListeners[0].OriginalDefaultPoolId)
	assert.Equal(t, usersPoolId, *task.lbConfig.Status.CreatedListeners[0].OriginalDefaultPoolId)
}
