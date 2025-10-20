package k8s_repo

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vlbv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/vlb/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func NewK8sRepository() repository.IK8sRepository {
	return &k8sRepository{}
}

type k8sRepository struct {
	// List(ctx context.Context, list ObjectList, opts ...ListOption) error
	client client.Client
}

func (r *k8sRepository) ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error {
	return r.client.List(ctx, list, opts...)
}

func (r *k8sRepository) CreateVLBC(ctx context.Context, vlbc *vlbv1alpha1.VngcloudLoadBalancerConfig) error {
	return nil
}
func (r *k8sRepository) DeleteVLBC(ctx context.Context, vlbc *vlbv1alpha1.VngcloudLoadBalancerConfig) error {
	return nil
}
