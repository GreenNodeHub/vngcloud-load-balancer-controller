package k8s_repo

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func NewK8sRepository(client client.Client) repository.IK8sRepository {
	return &k8sRepository{
		client: client,
	}
}

type k8sRepository struct {
	client client.Client
}

func (r *k8sRepository) ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error {
	return r.client.List(ctx, list, opts...)
}

func (r *k8sRepository) GetVLBC(ctx context.Context, n types.NamespacedName) (*v1alpha1.VngcloudLoadBalancerConfig, error) {
	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{}
	err := r.client.Get(ctx, n, vlbc)
	return vlbc, err
}

func (r *k8sRepository) CreateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error {
	return r.client.Create(ctx, vlbc)
}

func (r *k8sRepository) DeleteVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error {
	return r.client.Delete(ctx, vlbc)
}
