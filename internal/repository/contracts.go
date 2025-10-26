package repository

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IVngCloudRepository interface {
	// Load Balancer
	ListLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListLoadBalancers, error)
	GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error)
	GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error)
	CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lbID string) error
	ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error
	WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error)

	// Subnet
	GetSubnetByID(ctx context.Context, networkID, subnetID string) (*entityv2.Subnet, error)

	// Server
	GetServerNetworkInfo(ctx context.Context, serverID string) (zoneID common.Zone, networkId, subnetID, subnetCIDR string, err error)

	// Pool
	CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error)
	ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error)
	UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error
	GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error)
	DeletePool(ctx context.Context, lbID, poolID string) error
	UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error
	GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error)

	// Listener
	CreateListener(ctx context.Context, lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error)
	ListListenerOfLB(ctx context.Context, lbID string) (*entityv2.ListListeners, error)
	DeleteListener(ctx context.Context, lbID, listenerID string) error
	UpdateListener(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error
}

type IK8sRepository interface {
	GetService(ctx context.Context, n types.NamespacedName) (*corev1.Service, error)
	UpdateServiceStatusAddress(ctx context.Context, n types.NamespacedName, address string) error

	ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error
	// List(ctx context.Context, list ObjectList, opts ...ListOption) error

	GetVLBC(ctx context.Context, n types.NamespacedName) (*v1alpha1.VngcloudLoadBalancerConfig, error)
	CreateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, opts ...client.CreateOption) error
	DeleteVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error
	PatchVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, patch client.Patch, opts ...client.PatchOption) error
	UpdateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, opts ...client.UpdateOption) error
	PatchMutateStatusVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, mutateFunc func(ctx context.Context, obj *v1alpha1.VngcloudLoadBalancerConfig)) error
}
