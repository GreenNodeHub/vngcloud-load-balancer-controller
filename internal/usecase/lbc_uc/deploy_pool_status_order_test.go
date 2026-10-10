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
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// Status must never run ahead of the load balancer.
//
// mergePoolMembers decides whether a member is ours to remove by looking it up in
// status.createdPools[].createdMembers. A member that is neither in spec nor in that list is
// treated as somebody else's and kept forever. So if status is advanced to the new desired set
// before the members are actually pushed, and the push then fails, the member that was dropped
// from spec is no longer on our books - and no later pass will ever remove it.
//
// That is how 321 stale members accumulated on farm han: UpdatePoolMembers failed against
// MEMBER_PER_POOL quota while status had already moved on, leaving pools holding 84 members for a
// 38-node cluster.
func TestDeployPoolDoesNotRecordMembersItCouldNotPush(t *testing.T) {
	const (
		lbID   = "lb-1"
		poolID = "pool-1"
		name   = "p1"
	)
	keep := v1alpha1.PoolMember{Name: "node-a", IP: "10.0.0.1", Port: 80, MonitorPort: 80}
	gone := v1alpha1.PoolMember{Name: "node-b", IP: "10.0.0.2", Port: 80, MonitorPort: 80}

	// What the API server holds. The mock applies each mutation to it, so the assertions below
	// are about the recorded state, not about which calls were made.
	stored := &v1alpha1.LoadBalancerConfig{
		Status: v1alpha1.LoadBalancerConfigStatus{
			CreatedPools: []v1alpha1.CreatedPool{
				{Id: poolID, Name: name, CreatedMembers: []v1alpha1.PoolMember{keep, gone}},
			},
		},
	}

	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)

	k8s.EXPECT().
		PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ *v1alpha1.LoadBalancerConfig,
			mutate func(context.Context, *v1alpha1.LoadBalancerConfig) bool) error {
			mutate(ctx, stored)
			return nil
		}).Maybe()

	// deployPool reads the load balancer's tags on the match-by-name path, to tell a pool of the
	// cluster's from one of the user's that carries the same name. This one is the cluster's.
	vngcloud.EXPECT().
		ListTags(mock.Anything, lbID).
		Return(&entityv2.ListTags{Items: []*entityv2.Tag{
			{Key: "vng.vks.created-by-cluster", Value: "k8s-test"},
		}}, nil).Maybe()
	vngcloud.EXPECT().
		GetPoolHealthMonitorById(mock.Anything, lbID, poolID).
		Return(&entityv2.HealthMonitor{}, nil)
	vngcloud.EXPECT().
		UpdatePool(mock.Anything, lbID, poolID, mock.Anything).
		Return(nil).Maybe()
	vngcloud.EXPECT().
		WaitForLBActive(mock.Anything, lbID).
		Return(&entityv2.LoadBalancer{UUID: lbID}, nil).Maybe()

	// The load balancer still carries both members...
	vngcloud.EXPECT().
		GetPoolMembers(mock.Anything, lbID, poolID).
		Return(&entityv2.ListMembers{Items: []*entityv2.Member{
			{Name: keep.Name, Address: keep.IP, ProtocolPort: keep.Port, MonitorPort: keep.MonitorPort},
			{Name: gone.Name, Address: gone.IP, ProtocolPort: gone.Port, MonitorPort: gone.MonitorPort},
		}}, nil)

	// ...and the attempt to drop one of them fails, the way a quota rejection does.
	vngcloud.EXPECT().
		UpdatePoolMembers(mock.Anything, lbID, poolID, mock.Anything).
		Return(errors.New("Exceeded MEMBER_PER_POOL quota. Current used: 0, max: 50."))

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		cfg:          &config.Config{},
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				Pools: []v1alpha1.Pool{{Name: name, Members: []v1alpha1.PoolMember{keep}}},
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				CreatedPools: []v1alpha1.CreatedPool{
					{Id: poolID, Name: name, CreatedMembers: []v1alpha1.PoolMember{keep, gone}},
				},
			},
		},
	}

	spec := task.lbConfig.Spec.Pools[0]
	_, err := task.deployPool(context.Background(), lbID, &spec,
		&entityv2.ListPools{Items: []*entityv2.Pool{{UUID: poolID, Name: name}}})
	require.Error(t, err, "a failed member update must fail the pool")

	require.Len(t, stored.Status.CreatedPools, 1)
	recorded := stored.Status.CreatedPools[0].CreatedMembers
	ips := make([]string, 0, len(recorded))
	for _, m := range recorded {
		ips = append(ips, m.IP)
	}
	assert.Contains(t, ips, gone.IP,
		"status dropped a member that is still on the load balancer; the next pass will see it as "+
			"somebody else's and keep it forever")
}
