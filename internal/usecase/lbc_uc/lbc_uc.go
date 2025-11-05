package lbc_uc

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
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

	// Perform the actual reconciliation
	err = uc.ensure(ctx, lbConfig)

	// Update reconciliation tracking fields
	now := metav1.Now()
	message := "Successfully reconciled"
	if err != nil {
		message = err.Error()
	}

	// !IMPORTANT!: The tests will fail without this
	statusErr := uc.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.ObservedGeneration = &lbConfig.Generation
		obj.Status.LastReconcileTime = &now
		obj.Status.LastReconcileMessage = message
	})
	if statusErr != nil {
		logger := contexts.NewContext(ctx).Log()
		logger.Warnf("Failed to update reconciliation tracking fields: %v", statusErr)
	}

	return err
}

func (uc *lbcUseCase) ensure(ctx context.Context, lbConfig *v1alpha1.LoadBalancerConfig) error {
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
