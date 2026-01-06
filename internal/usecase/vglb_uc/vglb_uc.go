package vglb_uc

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

type vglbUseCase struct {
	cfg          *config.Config
	k8sRepo      repository.K8sRepository
	vngcloudRepo repository.VngCloudRepository
}

func NewVngcloudGlobalLoadBalancerUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
) usecase.VngcloudGlobalLoadBalancerUseCase {
	return &vglbUseCase{
		cfg:          cfg,
		k8sRepo:      k8sRepo,
		vngcloudRepo: vngcloudRepo,
	}
}

func (uc *vglbUseCase) InitVngcloudGlobalLoadBalancerUseCase(ctx context.Context) error {
	return nil
}

func (uc *vglbUseCase) EnsureVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error {
	vglb, err := uc.k8sRepo.GetVngcloudGlobalLoadBalancer(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Perform the actual reconciliation
	err = uc.ensure(ctx, vglb)

	return err
}

func (uc *vglbUseCase) ensure(ctx context.Context, vglb *v1alpha1.VngcloudGlobalLoadBalancer) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Ensuring VngcloudGlobalLoadBalancer %s/%s", vglb.Namespace, vglb.Name)

	// TODO: Implement the actual ensure logic for VngcloudGlobalLoadBalancer
	// This will involve creating/updating the global load balancer resources

	return nil
}

func (uc *vglbUseCase) DeleteVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error {
	vglb, err := uc.k8sRepo.GetVngcloudGlobalLoadBalancer(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Deleting VngcloudGlobalLoadBalancer %s/%s", vglb.Namespace, vglb.Name)

	// TODO: Implement the actual delete logic for VngcloudGlobalLoadBalancer
	// This will involve cleaning up the global load balancer resources

	return nil
}
