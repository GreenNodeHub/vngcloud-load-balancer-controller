package repository

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IVngCloudRepository interface {
	CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lbID string) error
}

type IK8sRepository interface {
	ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error
	// List(ctx context.Context, list ObjectList, opts ...ListOption) error

	GetVLBC(ctx context.Context, n types.NamespacedName) (*v1alpha1.VngcloudLoadBalancerConfig, error)
	CreateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error
	DeleteVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error
}
