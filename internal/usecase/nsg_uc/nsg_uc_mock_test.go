package nsg_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// helpers

func serverWithSecgroups(uuid string, sgUUIDs ...string) *entityv2.Server {
	sgs := make([]entityv2.ServerSecgroup, 0, len(sgUUIDs))
	for _, id := range sgUUIDs {
		sgs = append(sgs, entityv2.ServerSecgroup{Uuid: id})
	}
	return &entityv2.Server{Uuid: uuid, SecGroups: sgs}
}

func listServers(servers ...*entityv2.Server) *entityv2.ListServers {
	return &entityv2.ListServers{Items: servers}
}

func newUC(t *testing.T) (*nsgUseCase, *repository.MockVngCloudRepository) {
	t.Helper()
	vngcloud := repository.NewMockVngCloudRepository(t)
	uc := &nsgUseCase{vngcloudRepo: vngcloud}
	return uc, vngcloud
}

func nsgWith(serverSecgroups []v1alpha1.ServerSecurityGroupStatus) *v1alpha1.NodeSecurityGroup {
	return &v1alpha1.NodeSecurityGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-nsg", Namespace: "default"},
		Status: v1alpha1.NodeSecurityGroupStatus{
			ServerSecurityGroups: serverSecgroups,
		},
	}
}

// ── deleteManagedSecurityGroupIfUnused ──────────────────────────────────────

func TestDeleteManagedSecurityGroupIfUnused(t *testing.T) {
	ctx := context.Background()

	t.Run("secgroup not found — treated as already deleted", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(nil, fmt.Errorf("Cannot get security group with id secg-sg-1"))

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		require.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("get secgroup fails with non-404 error — returns error", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(nil, fmt.Errorf("network timeout"))

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		assert.Error(t, err)
		assert.False(t, deleted)
	})

	t.Run("secgroup still attached to servers — skip deletion", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(&entityv2.Secgroup{Id: "sg-1"}, nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-1").
			Return(listServers(serverWithSecgroups("ins-1", "sg-1")), nil)

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		require.NoError(t, err)
		assert.False(t, deleted)
	})

	t.Run("list servers fails — returns error", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(&entityv2.Secgroup{Id: "sg-1"}, nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-1").
			Return(nil, fmt.Errorf("api error"))

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		assert.Error(t, err)
		assert.False(t, deleted)
	})

	t.Run("no servers attached — deletes secgroup", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(&entityv2.Secgroup{Id: "sg-1"}, nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-1").
			Return(listServers(), nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-1").
			Return(nil)

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		require.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("delete fails — returns error", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(&entityv2.Secgroup{Id: "sg-1"}, nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-1").
			Return(listServers(), nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-1").
			Return(fmt.Errorf("delete failed"))

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		assert.Error(t, err)
		assert.False(t, deleted)
	})

	t.Run("nil server list treated as empty — deletes secgroup", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetSecurityGroup(ctx, "sg-1").
			Return(&entityv2.Secgroup{Id: "sg-1"}, nil)
		vng.EXPECT().ListServerBySecgroupID(ctx, "sg-1").
			Return(nil, nil)
		vng.EXPECT().DeleteSecurityGroup(ctx, "sg-1").
			Return(nil)

		deleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, "sg-1")
		require.NoError(t, err)
		assert.True(t, deleted)
	})
}

// ── ensureSecgroupForInstance ───────────────────────────────────────────────

func TestEnsureSecgroupForInstance(t *testing.T) {
	ctx := context.Background()

	t.Run("server not found — returns error", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(nil, fmt.Errorf("Cannot get server with id ins-1"))

		err := uc.ensureSecgroupForInstance(ctx, nsgWith(nil), "ins-1", []string{"sg-new"})
		assert.Error(t, err)
	})

	t.Run("no change needed — skips update", func(t *testing.T) {
		uc, vng := newUC(t)
		// current = [sg-1], old = [sg-1], new = [sg-1] → merged = [sg-1], no change
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1", "sg-1"), nil)

		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-1"}},
		})
		err := uc.ensureSecgroupForInstance(ctx, nsg, "ins-1", []string{"sg-1"})
		require.NoError(t, err)
		// UpdateSecGroupsOfServer must NOT be called (mock will fail if it is)
	})

	t.Run("add new secgroup — calls update and waits", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{"sg-new"}).
			Return(serverWithSecgroups("ins-1", "sg-new"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-1").
			Return(nil)

		err := uc.ensureSecgroupForInstance(ctx, nsgWith(nil), "ins-1", []string{"sg-new"})
		require.NoError(t, err)
	})

	t.Run("remove old secgroup, keep external", func(t *testing.T) {
		// current=[external, old-managed], old=[old-managed], new=[new-managed]
		// expected update call with [external, new-managed] (any order)
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1", "sg-external", "sg-old"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", matchElements("sg-external", "sg-new")).
			Return(serverWithSecgroups("ins-1", "sg-external", "sg-new"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-1").
			Return(nil)

		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-old"}},
		})
		err := uc.ensureSecgroupForInstance(ctx, nsg, "ins-1", []string{"sg-new"})
		require.NoError(t, err)
	})

	t.Run("detach all — empty desired list", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1", "sg-managed"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{}).
			Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-1").
			Return(nil)

		nsg := nsgWith([]v1alpha1.ServerSecurityGroupStatus{
			{ServerId: "ins-1", AttachedSecurityGroupIds: []string{"sg-managed"}},
		})
		err := uc.ensureSecgroupForInstance(ctx, nsg, "ins-1", []string{})
		require.NoError(t, err)
	})

	t.Run("update fails — returns error, no wait", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{"sg-new"}).
			Return(nil, fmt.Errorf("quota exceeded"))

		err := uc.ensureSecgroupForInstance(ctx, nsgWith(nil), "ins-1", []string{"sg-new"})
		assert.Error(t, err)
	})

	t.Run("wait fails — returns error", func(t *testing.T) {
		uc, vng := newUC(t)
		vng.EXPECT().GetServerByID(ctx, "ins-1").
			Return(serverWithSecgroups("ins-1"), nil)
		vng.EXPECT().UpdateSecGroupsOfServer(ctx, "ins-1", []string{"sg-new"}).
			Return(serverWithSecgroups("ins-1", "sg-new"), nil)
		vng.EXPECT().WaitForServerActive(ctx, "ins-1").
			Return(fmt.Errorf("timeout"))

		err := uc.ensureSecgroupForInstance(ctx, nsgWith(nil), "ins-1", []string{"sg-new"})
		assert.Error(t, err)
	})
}

// matchElements returns a mock.MatchedBy matcher that checks slice elements regardless of order.
func matchElements(expected ...string) interface{} {
	return mock.MatchedBy(func(actual []string) bool {
		if len(actual) != len(expected) {
			return false
		}
		seen := make(map[string]int)
		for _, s := range actual {
			seen[s]++
		}
		for _, s := range expected {
			if seen[s] == 0 {
				return false
			}
			seen[s]--
		}
		return true
	})
}
