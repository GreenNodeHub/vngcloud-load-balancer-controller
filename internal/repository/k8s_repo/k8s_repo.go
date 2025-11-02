package k8s_repo

import (
	"context"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func NewK8sRepository(client client.Client) repository.K8sRepository {
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

func (r *k8sRepository) GetLoadBalancerConfig(ctx context.Context, n types.NamespacedName) (*v1alpha1.LoadBalancerConfig, error) {
	lbc := &v1alpha1.LoadBalancerConfig{}
	err := r.client.Get(ctx, n, lbc)
	return lbc, err
}

func (r *k8sRepository) CreateLoadBalancerConfig(ctx context.Context, lbc *v1alpha1.LoadBalancerConfig, opts ...client.CreateOption) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Creating LBC %s/%s", lbc.Namespace, lbc.Name)
	return r.client.Create(ctx, lbc, opts...)
}

func (r *k8sRepository) DeleteLoadBalancerConfig(ctx context.Context, lbc *v1alpha1.LoadBalancerConfig) error {
	return r.client.Delete(ctx, lbc)
}

func (r *k8sRepository) PatchLoadBalancerConfig(ctx context.Context, lbc *v1alpha1.LoadBalancerConfig, patch client.Patch, opts ...client.PatchOption) error {
	return r.client.Patch(ctx, lbc, patch, opts...)
}

func (r *k8sRepository) UpdateLoadBalancerConfig(ctx context.Context, lbc *v1alpha1.LoadBalancerConfig, opts ...client.UpdateOption) error {
	return r.client.Update(ctx, lbc, opts...)
}

func (r *k8sRepository) PatchMutateStatusLoadBalancerConfig(
	ctx context.Context,
	lbc *v1alpha1.LoadBalancerConfig,
	mutate func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig),
) error {
	return r.patchMutateStatusObject(ctx, lbc, func(ctx context.Context, obj client.Object) {
		// type-assert so you can use strongly typed fields
		mutate(ctx, obj.(*v1alpha1.LoadBalancerConfig))
	})
}

func (r *k8sRepository) patchMutateStatusObject(
	ctx context.Context,
	obj client.Object,
	mutate func(ctx context.Context, obj client.Object),
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// get fresh copy
		objGet := obj.DeepCopyObject().(client.Object)
		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}, objGet); err != nil {
			return err
		}

		// deep copy for diff/patch base
		oldObject := objGet.DeepCopyObject().(client.Object)

		// mutate the fetched object (not the input)
		mutate(ctx, objGet)

		// patch only status, with optimistic lock
		return r.client.Status().Patch(ctx, objGet,
			client.MergeFromWithOptions(oldObject, client.MergeFromWithOptimisticLock{}))
	})
}

func (r *k8sRepository) ListLoadBalancerConfig(ctx context.Context, list *v1alpha1.LoadBalancerConfigList, opts ...client.ListOption) error {
	return r.client.List(ctx, list, opts...)
}

// NodeSecurityGroup methods would go here

func (r *k8sRepository) GetNodeSecurityGroup(ctx context.Context, n types.NamespacedName) (*v1alpha1.NodeSecurityGroup, error) {
	nsg := &v1alpha1.NodeSecurityGroup{}
	err := r.client.Get(ctx, n, nsg)
	return nsg, err
}

func (r *k8sRepository) ListNodeSecurityGroup(ctx context.Context, list *v1alpha1.NodeSecurityGroupList, opts ...client.ListOption) error {
	return r.client.List(ctx, list, opts...)
}

func (r *k8sRepository) CreateNodeSecurityGroup(ctx context.Context, nsg *v1alpha1.NodeSecurityGroup, opts ...client.CreateOption) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Creating NodeSecurityGroup %s/%s", nsg.Namespace, nsg.Name)
	return r.client.Create(ctx, nsg, opts...)
}

func (r *k8sRepository) DeleteNodeSecurityGroup(ctx context.Context, nsg *v1alpha1.NodeSecurityGroup) error {
	return r.client.Delete(ctx, nsg)
}

func (r *k8sRepository) PatchNodeSecurityGroup(ctx context.Context, nsg *v1alpha1.NodeSecurityGroup, patch client.Patch, opts ...client.PatchOption) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("%s Patching NodeSecurityGroup %s/%s", domain.RequestIcon, nsg.Namespace, nsg.Name)
	return r.client.Patch(ctx, nsg, patch, opts...)
}

func (r *k8sRepository) PatchMutateStatusNodeSecurityGroup(
	ctx context.Context,
	nsg *v1alpha1.NodeSecurityGroup,
	mutate func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup),
) error {
	return r.patchMutateStatusObject(ctx, nsg, func(ctx context.Context, obj client.Object) {
		// type-assert so you can use strongly typed fields
		mutate(ctx, obj.(*v1alpha1.NodeSecurityGroup))
	})
}
