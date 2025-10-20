package repository

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vlbv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/vlb/v1alpha1"
)

type IVngCloudRepository interface {
	CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lbID string) error
}

type IK8sRepository interface {
	ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error
	// List(ctx context.Context, list ObjectList, opts ...ListOption) error

	CreateVLBC(ctx context.Context, vlbc *vlbv1alpha1.VngcloudLoadBalancerConfig) error
	DeleteVLBC(ctx context.Context, vlbc *vlbv1alpha1.VngcloudLoadBalancerConfig) error
}
