package lbc_uc

import (
	"context"
	"sync"

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

	// lbLocks holds a mutex per LoadBalancer ID to prevent concurrent modifications
	// when multiple LBCs share the same LoadBalancer
	lbLocks sync.Map // map[string]*sync.Mutex
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

func (uc *lbcUseCase) InitLoadBalancerConfigUseCase(ctx context.Context) error {
	return nil
}

// getLBLock returns a mutex for the given LoadBalancer ID, creating one if necessary.
// This ensures only one LBC can modify a LoadBalancer at a time.
func (uc *lbcUseCase) getLBLock(lbID string) *sync.Mutex {
	if lbID == "" {
		return nil
	}
	lock, _ := uc.lbLocks.LoadOrStore(lbID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// getLoadBalancerID returns the LoadBalancer ID from spec or status
func (uc *lbcUseCase) getLoadBalancerID(lbConfig *v1alpha1.LoadBalancerConfig) string {
	if lbConfig.Spec.LoadBalancerId != nil && *lbConfig.Spec.LoadBalancerId != "" {
		return *lbConfig.Spec.LoadBalancerId
	}
	if lbConfig.Status.LoadBalancerId != nil && *lbConfig.Status.LoadBalancerId != "" {
		return *lbConfig.Status.LoadBalancerId
	}
	return ""
}

func (uc *lbcUseCase) EnsureLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error {
	lbConfig, err := uc.k8sRepo.GetLoadBalancerConfig(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Acquire lock by LoadBalancer ID to prevent concurrent modifications
	if lock := uc.getLBLock(uc.getLoadBalancerID(lbConfig)); lock != nil {
		lock.Lock()
		defer lock.Unlock()
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

func (uc *lbcUseCase) DeleteLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error {
	lbConfig, err := uc.k8sRepo.GetLoadBalancerConfig(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Acquire lock by LoadBalancer ID to prevent concurrent modifications
	if lock := uc.getLBLock(uc.getLoadBalancerID(lbConfig)); lock != nil {
		lock.Lock()
		defer lock.Unlock()
	}

	logger := contexts.NewContext(ctx).Log()
	task := &defaultModelDeployTask{
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
