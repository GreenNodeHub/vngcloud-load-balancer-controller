package lbc_uc

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

type lbcUseCase struct {
	cfg          *config.Config
	k8sRepo      repository.K8sRepository
	vngcloudRepo repository.VngCloudRepository
}

func NewLoadBalancerConfigUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
) usecase.LoadBalancerConfigUseCase {
	return &lbcUseCase{
		cfg:          cfg,
		k8sRepo:      k8sRepo,
		vngcloudRepo: vngcloudRepo,
	}
}

func (uc *lbcUseCase) Init(ctx context.Context) error {
	return nil
}

func (uc *lbcUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
	lbConfig, err := uc.k8sRepo.GetLoadBalancerConfig(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	task := &defaultModelDeployTask{
		logger:       logger,
		cfg:          uc.cfg,
		lbConfig:     lbConfig,
		vngcloudRepo: uc.vngcloudRepo,
		k8sRepo:      uc.k8sRepo,
	}

	if err := task.deploy(ctx); err != nil {
		return err
	}

	return nil
}

func (uc *lbcUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	lbConfig, err := uc.k8sRepo.GetLoadBalancerConfig(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	task := &defaultModelDeleteTask{
		logger:       logger,
		cfg:          uc.cfg,
		lbConfig:     lbConfig,
		vngcloudRepo: uc.vngcloudRepo,
		k8sRepo:      uc.k8sRepo,
	}

	if err := task.delete(ctx); err != nil {
		return err
	}

	return nil
}
