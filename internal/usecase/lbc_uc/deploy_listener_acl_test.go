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

const aclListenerId = "lis-acl"

func aclTask(vng *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository, spec v1alpha1.Listener, rec *v1alpha1.ListenerAcl) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          &config.Config{LoadBalancerOpts: config.LoadBalancerOpts{DefaultAllowedCidrs: "0.0.0.0/0"}},
		vngcloudRepo: vng,
		k8sRepo:      k8s,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{Type: loadbalancerv2.LoadBalancerTypeLayer4, Listeners: []v1alpha1.Listener{spec}},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId:        ptrTo("lb-acl"),
				CreatedLoadBalancerId: ptrTo("lb-acl"),
				CreatedListeners:      []v1alpha1.CreatedListener{{Id: aclListenerId, Port: 80, OriginalAcl: rec}},
			},
		},
	}
}

func onListener(vng *repository.MockVngCloudRepository, l entityv2.Listener) {
	l.UUID, l.ProtocolPort, l.Protocol = aclListenerId, 80, "TCP"
	vng.EXPECT().ListListenerOfLB(mock.Anything, "lb-acl").Return(&entityv2.ListListeners{Items: []*entityv2.Listener{&l}}, nil)
}

func tcp80(blocked *string) v1alpha1.Listener {
	return v1alpha1.Listener{Name: "l80", Protocol: loadbalancerv2.ListenerProtocolTCP, ProtocolPort: 80, BlockedCidrs: blocked}
}

// Review Focus 5: the record must be on the object before the PUT, or a crash after the PUT loses
// the only copy of what to put back.
func TestAclRecordIsWrittenBeforeThePut(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	applyStatusPatch(k8s)
	onListener(vng, entityv2.Listener{AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "192.0.2.1/32", DefaultAction: "accept"})
	task := aclTask(vng, k8s, tcp80(ptrTo("203.0.113.9/32")), nil)

	vng.EXPECT().UpdateListener(mock.Anything, "lb-acl", aclListenerId, mock.Anything).
		RunAndReturn(func(context.Context, string, string, loadbalancerv2.IUpdateListenerRequest) error {
			rec := task.originalAclOf(aclListenerId)
			require.NotNil(t, rec, "record must exist when the PUT is sent")
			assert.Equal(t, "192.0.2.1/32", *rec.BlockedCidrs)
			return errors.New("boom")
		}).Once()

	_, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.Error(t, err)
	assert.Equal(t, "192.0.2.1/32", *task.originalAclOf(aclListenerId).BlockedCidrs, "a failed PUT keeps the record")
}

// vLB keeps fields a PUT leaves out, so an unrelated update must not touch the ACL at all - sending
// the current value back would race a portal edit made between our read and our write.
func TestUnrelatedUpdateLeavesTheAclOut(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	applyStatusPatch(k8s)
	onListener(vng, entityv2.Listener{AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "203.0.113.8/32", DefaultAction: "drop", TimeoutClient: 50})
	spec := tcp80(nil)
	spec.TimeoutClient = ptrTo(int32(60))
	task := aclTask(vng, k8s, spec, nil)

	var sent *loadbalancerv2.UpdateListenerRequest
	vng.EXPECT().UpdateListener(mock.Anything, "lb-acl", aclListenerId, mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, o loadbalancerv2.IUpdateListenerRequest) error {
			sent = o.(*loadbalancerv2.UpdateListenerRequest)
			return nil
		}).Once()
	vng.EXPECT().WaitForLBActive(mock.Anything, "lb-acl").Return(&entityv2.LoadBalancer{UUID: "lb-acl"}, nil)

	_, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, sent.BlockedCidrs)
	assert.Nil(t, sent.DefaultAction)
}

func TestRemovingTheAnnotationRestoresAndReleases(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	applyStatusPatch(k8s)
	onListener(vng, entityv2.Listener{AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "203.0.113.9/32", DefaultAction: "accept"})
	task := aclTask(vng, k8s, tcp80(nil), &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")})

	var sent *loadbalancerv2.UpdateListenerRequest
	vng.EXPECT().UpdateListener(mock.Anything, "lb-acl", aclListenerId, mock.Anything).
		RunAndReturn(func(_ context.Context, _, _ string, o loadbalancerv2.IUpdateListenerRequest) error {
			sent = o.(*loadbalancerv2.UpdateListenerRequest)
			return nil
		}).Once()
	vng.EXPECT().WaitForLBActive(mock.Anything, "lb-acl").Return(&entityv2.LoadBalancer{UUID: "lb-acl"}, nil)

	created, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "", *sent.BlockedCidrs, "the empty original is sent explicitly to clear the list")
	assert.True(t, created[0].OriginalAcl.IsEmpty(), "released record must not be carried into status")
}

// Created by the controller: no gap, and the record is the neutral listener.
func TestCreatedListenerCarriesAclAndRecordsNeutral(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	applyStatusPatch(k8s)
	vng.EXPECT().ListListenerOfLB(mock.Anything, "lb-acl").Return(&entityv2.ListListeners{}, nil)
	task := aclTask(vng, k8s, tcp80(ptrTo("203.0.113.9/32")), nil)
	task.lbConfig.Status.CreatedListeners = nil

	vng.EXPECT().CreateListener(mock.Anything, "lb-acl", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, o loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
			assert.Equal(t, "203.0.113.9/32", o.ToRequestBody().(*loadbalancerv2.CreateListenerRequest).BlockedCidrs)
			return &entityv2.Listener{UUID: aclListenerId}, nil
		}).Once()
	vng.EXPECT().WaitForLBActive(mock.Anything, "lb-acl").Return(&entityv2.LoadBalancer{UUID: "lb-acl"}, nil)
	vng.EXPECT().GetListenerById(mock.Anything, "lb-acl", aclListenerId).Return(&entityv2.Listener{
		UUID: aclListenerId, ProtocolPort: 80, Protocol: "TCP", AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "203.0.113.9/32", DefaultAction: "accept",
	}, nil)

	created, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, created[0].OriginalAcl)
	assert.Equal(t, "", *created[0].OriginalAcl.BlockedCidrs)
}

// storedStatusPatch models the real patch helper: the mutation runs against a fresh copy of the
// stored object and never touches the one passed in. It returns the stored object, so a test can
// assert on what was persisted, and a count of the patches that wrote.
func storedStatusPatch(k8s *repository.MockK8sRepository, task *defaultModelDeployTask) (*v1alpha1.LoadBalancerConfig, *int) {
	stored, writes := task.lbConfig.DeepCopy(), new(int)
	k8s.EXPECT().PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ *v1alpha1.LoadBalancerConfig,
			mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			fresh := stored.DeepCopy()
			if mutate(ctx, fresh) {
				*stored = *fresh
				*writes++
			}
			return nil
		})
	return stored, writes
}

// The neutral record written with a new listener must reach the value deployListener returns:
// deploy() replaces status.createdListeners with exactly those values, and the real patch helper
// leaves the task's own copy untouched.
func TestAclRecordOfACreatedListenerSurvivesAFreshCopyPatch(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	vng.EXPECT().ListListenerOfLB(mock.Anything, "lb-acl").Return(&entityv2.ListListeners{}, nil)
	task := aclTask(vng, k8s, tcp80(ptrTo("203.0.113.9/32")), nil)
	task.lbConfig.Status.CreatedListeners = nil
	stored, _ := storedStatusPatch(k8s, task)

	vng.EXPECT().CreateListener(mock.Anything, "lb-acl", mock.Anything).Return(&entityv2.Listener{UUID: aclListenerId}, nil).Once()
	vng.EXPECT().WaitForLBActive(mock.Anything, "lb-acl").Return(&entityv2.LoadBalancer{UUID: "lb-acl"}, nil)
	vng.EXPECT().GetListenerById(mock.Anything, "lb-acl", aclListenerId).Return(&entityv2.Listener{
		UUID: aclListenerId, ProtocolPort: 80, Protocol: "TCP", AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "203.0.113.9/32", DefaultAction: "accept",
	}, nil)

	created, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, created[0].OriginalAcl, "the record must be carried into the value deploy() writes back")
	assert.Equal(t, "", *created[0].OriginalAcl.BlockedCidrs)
	require.Len(t, stored.Status.CreatedListeners, 1)
	assert.Equal(t, "", *stored.Status.CreatedListeners[0].OriginalAcl.BlockedCidrs, "recorded with the listener")
}

// The original is already back on the listener (a portal edit, or a restore whose release was
// never written), so no PUT is needed - but the record must still be released.
func TestAclRecordIsReleasedWithoutAPut(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	onListener(vng, entityv2.Listener{AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "192.0.2.1/32", DefaultAction: "accept"})
	task := aclTask(vng, k8s, tcp80(nil), &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/32")})
	stored, _ := storedStatusPatch(k8s, task)

	created, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, created[0].OriginalAcl)
	assert.Nil(t, task.originalAclOf(aclListenerId))
	assert.Nil(t, stored.Status.CreatedListeners[0].OriginalAcl, "the release must be persisted")
}

// In the steady state - annotation applied, record held - a reconcile writes no ACL record.
func TestAclRecordUnchangedIsNotRewritten(t *testing.T) {
	vng, k8s := repository.NewMockVngCloudRepository(t), repository.NewMockK8sRepository(t)
	onListener(vng, entityv2.Listener{AllowedCidrs: "0.0.0.0/0", BlockedCidrs: "203.0.113.9/32", DefaultAction: "accept"})
	task := aclTask(vng, k8s, tcp80(ptrTo("203.0.113.9/32")), &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/32")})
	_, writes := storedStatusPatch(k8s, task)

	created, err := task.deployListeners(context.Background(), "lb-acl", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, *writes)
	assert.Equal(t, "192.0.2.1/32", *created[0].OriginalAcl.BlockedCidrs)
}

// The first listener of a load balancer this controller creates is created inline with the load
// balancer, so deployListener finds it by port and never reaches the create branch that writes the
// neutral record. Without the record here, removing the annotation would leave it blocked forever.
func TestListenerCreatedWithOurLoadBalancerGetsTheNeutralRecord(t *testing.T) {
	k8s := repository.NewMockK8sRepository(t)
	task := aclTask(nil, k8s, tcp80(ptrTo("203.0.113.9/32")), nil)
	task.lbConfig.Status.CreatedListeners = nil
	stored, _ := storedStatusPatch(k8s, task)

	require.NoError(t, task.statusAdoptListener(context.Background(), aclListenerId, 80, ""))

	require.Len(t, stored.Status.CreatedListeners, 1)
	got := stored.Status.CreatedListeners[0]
	assert.False(t, got.Adopted)
	require.NotNil(t, got.OriginalAcl)
	require.NotNil(t, got.OriginalAcl.BlockedCidrs)
	assert.Equal(t, "", *got.OriginalAcl.BlockedCidrs, "recorded in the persisted status")
	assert.Equal(t, got, task.lbConfig.Status.CreatedListeners[0], "the in-memory entry matches what was persisted")
}

func TestListenerCreatedWithOurLoadBalancerHasNoRecordWithoutAcl(t *testing.T) {
	k8s := repository.NewMockK8sRepository(t)
	task := aclTask(nil, k8s, tcp80(nil), nil)
	task.lbConfig.Status.CreatedListeners = nil
	stored, _ := storedStatusPatch(k8s, task)

	require.NoError(t, task.statusAdoptListener(context.Background(), aclListenerId, 80, ""))

	require.Len(t, stored.Status.CreatedListeners, 1)
	assert.Nil(t, stored.Status.CreatedListeners[0].OriginalAcl)
	assert.Nil(t, task.lbConfig.Status.CreatedListeners[0].OriginalAcl)
}

// A listener found on someone else's load balancer is adopted, and its ACL is left alone: the
// record only ever covers values this controller displaced.
func TestAdoptedListenerGetsNoNeutralRecord(t *testing.T) {
	k8s := repository.NewMockK8sRepository(t)
	task := aclTask(nil, k8s, tcp80(ptrTo("203.0.113.9/32")), nil)
	task.lbConfig.Status.CreatedListeners = nil
	task.lbConfig.Status.CreatedLoadBalancerId = nil
	task.lbConfig.Status.AdoptedLoadBalancerId = ptrTo("lb-acl")
	stored, _ := storedStatusPatch(k8s, task)

	require.NoError(t, task.statusAdoptListener(context.Background(), aclListenerId, 80, "pool-users"))

	require.Len(t, stored.Status.CreatedListeners, 1)
	got := stored.Status.CreatedListeners[0]
	assert.True(t, got.Adopted)
	assert.Nil(t, got.OriginalAcl)
	require.NotNil(t, got.OriginalDefaultPoolId)
	assert.Equal(t, "pool-users", *got.OriginalDefaultPoolId)
}
