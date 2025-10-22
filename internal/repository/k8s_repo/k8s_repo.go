package k8s_repo

import (
	"context"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/anngdinh/operator-helper/contexts"
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

func (r *k8sRepository) GetService(ctx context.Context, n types.NamespacedName) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := r.client.Get(ctx, n, svc)
	return svc, err
}

func (r *k8sRepository) UpdateServiceStatusAddress(ctx context.Context, n types.NamespacedName, address string) error {
	if address == "" {
		return nil
	}

	// Update the service status with the new address
	svc := &corev1.Service{}
	err := r.client.Get(ctx, n, svc)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	objectOld := svc.DeepCopy()

	addr := net.ParseIP(address)
	if addr != nil {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: address + ".nip.io"}}
	} else {
		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: address}}
	}
	return r.client.Status().Patch(ctx, svc, client.MergeFrom(objectOld))
}

func (r *k8sRepository) ListNode(ctx context.Context, list *corev1.NodeList, opts ...client.ListOption) error {
	return r.client.List(ctx, list, opts...)
}

func (r *k8sRepository) GetVLBC(ctx context.Context, n types.NamespacedName) (*v1alpha1.VngcloudLoadBalancerConfig, error) {
	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{}
	err := r.client.Get(ctx, n, vlbc)
	return vlbc, err
}

func (r *k8sRepository) CreateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, opts ...client.CreateOption) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Creating VLBC %s/%s", vlbc.Namespace, vlbc.Name)
	return r.client.Create(ctx, vlbc, opts...)
}

func (r *k8sRepository) DeleteVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig) error {
	return r.client.Delete(ctx, vlbc)
}

func (r *k8sRepository) PatchVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, patch client.Patch, opts ...client.PatchOption) error {
	return r.client.Patch(ctx, vlbc, patch, opts...)
}

func (r *k8sRepository) UpdateVLBC(ctx context.Context, vlbc *v1alpha1.VngcloudLoadBalancerConfig, opts ...client.UpdateOption) error {
	return r.client.Update(ctx, vlbc, opts...)
}
