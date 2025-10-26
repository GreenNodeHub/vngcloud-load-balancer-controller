package vlbc_uc

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

type vlbcUseCase struct {
	cfg          *config.Config
	k8sRepo      repository.IK8sRepository
	vngcloudRepo repository.IVngCloudRepository
}

func NewVLBCUseCase(
	cfg *config.Config,
	k8sRepo repository.IK8sRepository,
	vngcloudRepo repository.IVngCloudRepository,
) usecase.IVLBConfigUseCase {
	return &vlbcUseCase{
		cfg:          cfg,
		k8sRepo:      k8sRepo,
		vngcloudRepo: vngcloudRepo,
	}
}

func (uc *vlbcUseCase) Init(ctx context.Context) error {
	return nil
}

func (uc *vlbcUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
	err := uc.ensure(ctx, req)
	return err
}

func (uc *vlbcUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	return nil
}

//////////////////////////////////////////////////////////////////////////

func (uc *vlbcUseCase) ensure(ctx context.Context, req ctrl.Request) error {
	vlbConfig, err := uc.k8sRepo.GetVLBC(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	task := &defaultModelDeployTask{
		logger:       logger,
		cfg:          uc.cfg,
		vlbConfig:    vlbConfig,
		vngcloudRepo: uc.vngcloudRepo,
		k8sRepo:      uc.k8sRepo,
	}

	if err := task.deploy(ctx); err != nil {
		return err
	}

	return nil
}
