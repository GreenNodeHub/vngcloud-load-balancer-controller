package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// emptyLoadBalancer mocks the reads for a load balancer with nothing on it, so
// canDeleteWholeLoadBalancer says yes and every delete decision comes down to ownership.
func emptyLoadBalancer(vngcloud *repository.MockVngCloudRepository) {
	vngcloud.EXPECT().GetLoadBalancerByID(mock.Anything, "lb-1").
		Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).Maybe()
	vngcloud.EXPECT().ListListenerOfLB(mock.Anything, "lb-1").
		Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil).Maybe()
	vngcloud.EXPECT().ListPool(mock.Anything, "lb-1").
		Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil).Maybe()
	vngcloud.EXPECT().ListTags(mock.Anything, "lb-1").
		Return(&entityv2.ListTags{Items: []*entityv2.Tag{}}, nil).Maybe()
	vngcloud.EXPECT().CreateTags(mock.Anything, "lb-1", mock.Anything).
		Return(nil).Maybe()
}

func ownershipTask(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository, specLbId *string) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloud,
		k8sRepo:      k8s,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			Spec: v1alpha1.LoadBalancerConfigSpec{
				Type:           loadbalancerv2.LoadBalancerTypeLayer7,
				LoadBalancerId: specLbId,
				ClusterId:      ptr.To("cluster-1"),
			},
			Status: v1alpha1.LoadBalancerConfigStatus{
				LoadBalancerId: ptr.To("lb-1"),
			},
		},
	}
}

// A load balancer the user pinned with vks.vngcloud.vn/load-balancer-id is not ours to
// delete. It may still serve other LoadBalancerConfigs, or workloads outside this cluster
// entirely - a shared ALB was destroyed this way once, taking its IP with it and leaving
// every service on it dark until someone repointed them by hand.
//
// The mock is strict, so not expecting DeleteLoadBalancer is the assertion: any call fails
// the test.
func TestDeleteLoadBalancerNeverDeletesAUserProvidedLoadBalancer(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud)
	k8s.EXPECT().PatchMutateStatusLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Maybe()

	task := ownershipTask(vngcloud, k8s, ptr.To("lb-1")) // pinned by the user

	require.NoError(t, task.deleteLoadBalancer(context.Background(), "lb-1"))
}

// The other half of the boundary: a load balancer the controller created is still cleaned
// up. Without this case the change could disable load balancer deletion outright and the
// suite would stay green.
func TestDeleteLoadBalancerStillDeletesOneItCreated(t *testing.T) {
	vngcloud := repository.NewMockVngCloudRepository(t)
	k8s := repository.NewMockK8sRepository(t)
	emptyLoadBalancer(vngcloud)

	deleted := false
	vngcloud.EXPECT().DeleteLoadBalancer(mock.Anything, "lb-1").
		RunAndReturn(func(context.Context, string) error { deleted = true; return nil }).Once()

	task := ownershipTask(vngcloud, k8s, nil) // no annotation - the controller made it

	require.NoError(t, task.deleteLoadBalancer(context.Background(), "lb-1"))
	assert.True(t, deleted, "a load balancer the controller created must still be removed")
}
