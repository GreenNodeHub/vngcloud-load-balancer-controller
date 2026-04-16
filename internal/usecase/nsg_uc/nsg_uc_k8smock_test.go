package nsg_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newFullUC(t *testing.T) (*nsgUseCase, *repository.MockVngCloudRepository, *repository.MockK8sRepository) {
	t.Helper()
	vng := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	uc := &nsgUseCase{vngcloudRepo: vng, k8sRepo: k8s}
	return uc, vng, k8s
}

// nsgWithManagedID creates a test NSG whose status already records a managed secgroup ID.
func nsgWithManagedID(id string) *v1alpha1.NodeSecurityGroup {
	nsg := nsgWith(nil)
	nsg.Status.ManagedSecurityGroup.Id = ptr.To(id)
	return nsg
}

func listSecgroups(sgs ...*entityv2.Secgroup) *entityv2.ListSecgroups {
	return &entityv2.ListSecgroups{Items: sgs}
}

func makeSG(id, name string) *entityv2.Secgroup {
	return &entityv2.Secgroup{Id: id, Name: name}
}

func listRules(rules ...*entityv2.SecgroupRule) *entityv2.ListSecgroupRules {
	return &entityv2.ListSecgroupRules{Items: rules}
}

// expectPatch sets up a PatchMutateStatusNodeSecurityGroup expectation that actually
// executes the mutate closure, so the closure body is counted for coverage.
func expectPatch(k8s *repository.MockK8sRepository, ctx context.Context) {
	k8s.EXPECT().
		PatchMutateStatusNodeSecurityGroup(ctx, mock.Anything, mock.Anything).
		Run(func(ctx context.Context, nsg *v1alpha1.NodeSecurityGroup, f func(context.Context, *v1alpha1.NodeSecurityGroup) bool) {
			f(ctx, nsg)
		}).
		Return(nil)
}

// ── TestDoEnsureManagedSecurityGroup ─────────────────────────────────────────

func TestDoEnsureManagedSecurityGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("spec nil — returns empty status, no VNG calls", func(t *testing.T) {
		uc, _, _ := newFullUC(t)
		nsg := nsgWith(nil) // no Spec.ManagedSecurityGroup

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		assert.Nil(t, status.Id)
	})

	t.Run("list secgroups fails — returns error", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{Name: "sg-test"}
		vng.EXPECT().ListSecurityGroups(ctx).Return(nil, fmt.Errorf("api down"))

		_, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		assert.Error(t, err)
	})

	t.Run("secgroup found, rules already match — no create or delete", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{
			Name:  "sg-test",
			Rules: []v1alpha1.NodeSecurityGroupRule{desiredRule("0.0.0.0/0", 80, 80)},
		}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(makeSG("sg-1", "sg-test")), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-1").Return(
			listRules(ingressRule("r1", 80, 80)), nil)
		// no DeleteSecurityGroupRule or CreateSecurityGroupRule expected

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		require.NotNil(t, status.Id)
		assert.Equal(t, "sg-1", *status.Id)
	})

	t.Run("secgroup not found — creates, fetches, reconciles rules", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{Name: "sg-new"}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(), nil)
		vng.EXPECT().CreateSecurityGroup(ctx, "sg-new", mock.Anything).Return(makeSG("sg-created", "sg-new"), nil)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-created").Return(makeSG("sg-created", "sg-new"), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-created").Return(listRules(), nil)

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		require.NotNil(t, status.Id)
		assert.Equal(t, "sg-created", *status.Id)
	})

	t.Run("create secgroup fails — returns error", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{Name: "sg-new"}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(), nil)
		vng.EXPECT().CreateSecurityGroup(ctx, "sg-new", mock.Anything).Return(nil, fmt.Errorf("quota exceeded"))

		_, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		assert.Error(t, err)
	})

	t.Run("list rules fails — returns error", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{Name: "sg-test"}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(makeSG("sg-1", "sg-test")), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-1").Return(nil, fmt.Errorf("rules fetch failed"))

		_, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		assert.Error(t, err)
	})

	t.Run("extra current ingress rule — deletes it", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{
			Name:  "sg-test",
			Rules: []v1alpha1.NodeSecurityGroupRule{}, // no desired ingress rules
		}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(makeSG("sg-1", "sg-test")), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-1").Return(
			listRules(ingressRule("r1", 22, 22)), nil)
		vng.EXPECT().DeleteSecurityGroupRule(ctx, "sg-1", "r1").Return(nil)

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		assert.Equal(t, "sg-1", *status.Id)
	})

	t.Run("missing desired rule — creates it", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{
			Name:  "sg-test",
			Rules: []v1alpha1.NodeSecurityGroupRule{desiredRule("0.0.0.0/0", 443, 443)},
		}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(makeSG("sg-1", "sg-test")), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-1").Return(listRules(), nil)
		vng.EXPECT().CreateSecurityGroupRule(ctx, "sg-1", mock.Anything).Return(&entityv2.SecgroupRule{Id: "r-new"}, nil)

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		assert.Equal(t, "sg-1", *status.Id)
	})

	t.Run("egress rules not in desired are deleted — controller owns all rules", func(t *testing.T) {
		uc, vng, _ := newFullUC(t)
		nsg := nsgWith(nil)
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{
			Name:  "sg-test",
			Rules: []v1alpha1.NodeSecurityGroupRule{},
		}
		vng.EXPECT().ListSecurityGroups(ctx).Return(listSecgroups(makeSG("sg-1", "sg-test")), nil)
		vng.EXPECT().ListSecurityGroupRules(ctx, "sg-1").Return(
			listRules(egressRule("egress-1"), egressRule("egress-2")), nil)
		vng.EXPECT().DeleteSecurityGroupRule(ctx, "sg-1", "egress-1").Return(nil)
		vng.EXPECT().DeleteSecurityGroupRule(ctx, "sg-1", "egress-2").Return(nil)

		status, err := uc.doEnsureManagedSecurityGroup(ctx, nsg)
		require.NoError(t, err)
		assert.Equal(t, "sg-1", *status.Id)
	})
}

// ── TestEnsureServerSecurityGroups ───────────────────────────────────────────

func TestEnsureServerSecurityGroups(t *testing.T) {
	ctx := context.Background()

	t.Run("no servers, no old managed ID — returns empty, no calls", func(t *testing.T) {
		uc, _, _ := newFullUC(t)
		nsg := nsgWith(nil)

		err := uc.ensureServerSecurityGroups(ctx, nsg, nil, v1alpha1.ManagedSecurityGroupStatus{})
		require.NoError(t, err)
	})

	t.Run("notChange server, already has managed secgroup — status upserted, no cloud update", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-m"}},
		})
		nodeInfos := []v1alpha1.NodeInfo{{Name: "node-1", ServerId: "ins-1"}}
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-m")}

		// server already has sg-m and old status also tracks sg-m → mergeStringArray reports no change
		vng.EXPECT().GetServerByID(ctx, "ins-1").Return(serverWithSecgroups("ins-1", "sg-m"), nil)
		expectPatch(k8s, ctx) // statusUpdateNodeSecurityGroup called immediately after ensure

		err := uc.ensureServerSecurityGroups(ctx, nsg, nodeInfos, managedStatus)
		require.NoError(t, err)
	})

	t.Run("add new server — calls update, wait, then status upserted", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		nsg := nsgWith(nil)
		nodeInfos := []v1alpha1.NodeInfo{{Name: "node-new", ServerId: "ins-new"}}
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-m")}

		vng.EXPECT().GetServerByID(ctx, "ins-new").Return(serverWithSecgroups("ins-new"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-new", []string{"sg-m"}).Return(serverWithSecgroups("ins-new", "sg-m"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-new").Return(nil)
		expectPatch(k8s, ctx) // statusUpdateNodeSecurityGroup called immediately after ensure

		err := uc.ensureServerSecurityGroups(ctx, nsg, nodeInfos, managedStatus)
		require.NoError(t, err)
	})

	t.Run("remove server — detaches all secgroups, then status entry removed", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-old", AttachedSecurityGroupIds: []string{"sg-m"}},
		})
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-m")}

		vng.EXPECT().GetServerByID(ctx, "ins-old").Return(serverWithSecgroups("ins-old", "sg-m"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-old", []string{}).Return(serverWithSecgroups("ins-old"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-old").Return(nil)
		expectPatch(k8s, ctx) // statusRemoveServerSecurityGroup called immediately after ensure

		err := uc.ensureServerSecurityGroups(ctx, nsg, nil, managedStatus)
		require.NoError(t, err)
	})

	t.Run("remove server — server not found, treated as success, status entry removed", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-gone", AttachedSecurityGroupIds: []string{"sg-m"}},
		})

		vng.EXPECT().GetServerByID(ctx, "ins-gone").Return(nil, fmt.Errorf("Cannot get server with id ins-gone"))
		expectPatch(k8s, ctx) // statusRemoveServerSecurityGroup called on IsServerNotFound

		err := uc.ensureServerSecurityGroups(ctx, nsg, nil, v1alpha1.ManagedSecurityGroupStatus{})
		require.NoError(t, err)
	})

	t.Run("notChange server, update fails — error returned, status upserted with error field", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{}},
		})
		nodeInfos := []v1alpha1.NodeInfo{{Name: "node-1", ServerId: "ins-1"}}
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-m")}

		vng.EXPECT().GetServerByID(ctx, "ins-1").Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{"sg-m"}).Return(nil, fmt.Errorf("quota exceeded"))
		expectPatch(k8s, ctx) // statusUpdateNodeSecurityGroup called with error recorded

		err := uc.ensureServerSecurityGroups(ctx, nsg, nodeInfos, managedStatus)
		assert.Error(t, err)
	})

	t.Run("managed secgroup ID changed — old secgroup deleted, no extra status patch", func(t *testing.T) {
		// ensureManagedSecurityGroup (called before ensureServerSecurityGroups) already wrote
		// the new ID to status. ensureServerSecurityGroups must NOT overwrite it with nil.
		uc, vng, _ := newFullUC(t)
		nsg := nsgWithManagedID("sg-old")
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-new")}

		vng.EXPECT().GetSecurityGroup(ctx, "sg-old").Return(makeSG("sg-old", "old"), nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-old").Return(listServers(), nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-old").Return(nil)
		// no expectPatch — the correct new ID was already written by ensureManagedSecurityGroup

		err := uc.ensureServerSecurityGroups(ctx, nsg, nil, managedStatus)
		require.NoError(t, err)
	})

	t.Run("managed secgroup ID same — no cleanup attempt", func(t *testing.T) {
		uc, _, _ := newFullUC(t)
		nsg := nsgWithManagedID("sg-same")
		managedStatus := v1alpha1.ManagedSecurityGroupStatus{Id: ptr.To("sg-same")}

		err := uc.ensureServerSecurityGroups(ctx, nsg, nil, managedStatus)
		require.NoError(t, err)
	})
}

// ── TestDeleteNodeSecurityGroupUseCase ───────────────────────────────────────

func TestDeleteNodeSecurityGroupUseCase(t *testing.T) {
	ctx := context.Background()

	t.Run("NSG not found — returns nil", func(t *testing.T) {
		uc, _, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg-gone", Namespace: "default"}}
		notFound := k8serrors.NewNotFound(
			schema.GroupResource{Group: "networking.vngcloud.vn", Resource: "nodesecuritygroups"}, "nsg-gone")
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nil, notFound)

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		require.NoError(t, err)
	})

	t.Run("no servers, no managed secgroup — no-op", func(t *testing.T) {
		uc, _, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsgWith(nil), nil)

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		require.NoError(t, err)
	})

	t.Run("detach server — success", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-m"}},
		})
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsg, nil)

		vng.EXPECT().GetServerByID(ctx, "ins-1").Return(serverWithSecgroups("ins-1", "sg-m"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{}).Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-1").Return(nil)

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		require.NoError(t, err)
	})

	t.Run("detach fails with non-404 error — returns error", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-m"}},
		})
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsg, nil)
		vng.EXPECT().GetServerByID(ctx, "ins-1").Return(nil, fmt.Errorf("network error"))

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		assert.Error(t, err)
	})

	t.Run("server not found during detach — ignored", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-gone", AttachedSecurityGroupIds: []string{"sg-m"}},
		})
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsg, nil)
		vng.EXPECT().GetServerByID(ctx, "ins-gone").Return(nil, fmt.Errorf("Cannot get server with id ins-gone"))

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		require.NoError(t, err)
	})

	t.Run("has managed secgroup — deletion succeeds", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		nsg := nsgWithManagedID("sg-m")
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsg, nil)

		vng.EXPECT().GetSecurityGroup(ctx, "sg-m").Return(makeSG("sg-m", "managed"), nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-m").Return(listServers(), nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-m").Return(nil)

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		require.NoError(t, err)
	})

	t.Run("has managed secgroup — deletion fails", func(t *testing.T) {
		uc, vng, k8s := newFullUC(t)
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "nsg", Namespace: "default"}}
		nsg := nsgWithManagedID("sg-m")
		k8s.EXPECT().GetNodeSecurityGroup(ctx, req.NamespacedName).Return(nsg, nil)

		vng.EXPECT().GetSecurityGroup(ctx, "sg-m").Return(makeSG("sg-m", "managed"), nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-m").Return(listServers(), nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-m").Return(fmt.Errorf("delete failed"))

		err := uc.DeleteNodeSecurityGroupUseCase(ctx, req)
		assert.Error(t, err)
	})
}
