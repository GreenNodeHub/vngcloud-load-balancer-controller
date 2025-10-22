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
	CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lbID string) error

	GetSubnetByID(ctx context.Context, networkID, subnetID string) (*entityv2.Subnet, error)
	GetServerNetworkInfo(ctx context.Context, serverID string) (zoneID common.Zone, networkId, subnetID, subnetCIDR string, err error)
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
}
