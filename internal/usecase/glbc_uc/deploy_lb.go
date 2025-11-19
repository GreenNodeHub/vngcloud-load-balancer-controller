package glbc_uc

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)

type defaultModelDeployTask struct {
	logger       *logrus.Entry
	cfg          *config.Config
	vngcloudRepo repository.VngCloudRepository
	k8sRepo      repository.K8sRepository
	lbConfig     *v1alpha1.GlobalLoadBalancerConfig
}

func (t *defaultModelDeployTask) deploy(ctx context.Context) error {
	// LAYER 1: Validate the GlobalLoadBalancerConfig itself (internal consistency)
	// This runs before we even get the load balancer ID
	if err := t.validateSelf(ctx); err != nil {
		return err
	}

	lbId, err := t.deployLoadBalancer(ctx)
	if err != nil {
		return err
	}

	// LAYER 2: Validate across GlobalLoadBalancerConfigs sharing the same load balancer
	// This runs after we have the load balancer ID
	if err := t.validateCrossGLBCs(ctx, lbId); err != nil {
		return err
	}

	newCreatedPools, err := t.deployPools(ctx, lbId)
	if err != nil {
		return err
	}
	newCreatedListeners, err := t.deployListeners(ctx, lbId, newCreatedPools)
	if err != nil {
		return err
	}

	err = t.deleteRedundantListeners(ctx, lbId, newCreatedListeners, newCreatedPools)
	if err != nil {
		return err
	}
	err = t.deleteRedundantPools(ctx, lbId, newCreatedPools)
	if err != nil {
		return err
	}

	// update status
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		obj.Status.CreatedListeners = newCreatedListeners
		obj.Status.CreatedPools = newCreatedPools
	})
}

func (t *defaultModelDeployTask) deployLoadBalancer(ctx context.Context) (string, error) {
	// if already an exist lb
	if t.lbConfig.Status.LoadBalancerId != nil && *t.lbConfig.Status.LoadBalancerId != "" {
		if t.lbConfig.Spec.LoadBalancerId != nil && *t.lbConfig.Spec.LoadBalancerId != "" && *t.lbConfig.Spec.LoadBalancerId != *t.lbConfig.Status.LoadBalancerId {
			return t.migrateLoadBalancer(ctx, *t.lbConfig.Status.LoadBalancerId, *t.lbConfig.Spec.LoadBalancerId)
		} else {
			return t.ensureExistLoadBalancer(ctx, *t.lbConfig.Status.LoadBalancerId, nil)
		}
	}

	// if not exist lbId in status
	// try to use spec lbId
	if t.lbConfig.Spec.LoadBalancerId != nil && *t.lbConfig.Spec.LoadBalancerId != "" {
		return t.ensureExistLoadBalancer(ctx, *t.lbConfig.Spec.LoadBalancerId, nil)
	}

	// try to use spec lb Name
	if t.lbConfig.Spec.Name != "" {
		lb, err := t.vngcloudRepo.GetGlobalLoadBalancerByName(ctx, t.lbConfig.Spec.Name)
		if err != nil {
			return "", err
		}
		if lb != nil {
			return t.ensureExistLoadBalancer(ctx, lb.ID, lb)
		}
	}

	return t.createLoadBalancer(ctx)
}

func (t *defaultModelDeployTask) createLoadBalancer(ctx context.Context) (string, error) {
	var lbEntity *entity.GlobalLoadBalancer
	var err error

	createRequest, err := t.buildCreateLoadBalancerRequest(ctx)
	if err != nil {
		return "", err
	}
	lbEntity, err = t.vngcloudRepo.CreateGlobalLoadBalancer(ctx, createRequest)
	if err != nil {
		return "", err
	}
	if lbEntity == nil || lbEntity.ID == "" {
		return "", errors.New("load balancer not have ID after create, need to retry")
	}

	// wait for loadbalancer active, if lb is error, delete it and return error
	if _, err = t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbEntity.ID); err != nil {
		if err == domain.ErrorLoadBalancerStatusError {
			if err := t.vngcloudRepo.DeleteGlobalLoadBalancer(ctx, lbEntity.ID); err != nil {
				t.logger.Error("Failed to delete loadbalancer: ", err)
				return "", err
			}
			t.logger.Infof("Delete loadbalancer \"%s\" because of status error, recreate now.", lbEntity.ID)
			return "", errs.NewRequeueNeeded("loadbalancer status is error, delete and recreate")
		}
		t.logger.Error("Failed to wait for loadbalancer active: ", err)
		return "", err
	}

	t.logger.Infof("Created load balancer with ID %s", lbEntity.ID)
	return t.ensureExistLoadBalancer(ctx, lbEntity.ID, nil)
}

// when update load balancer id to new value
func (t *defaultModelDeployTask) migrateLoadBalancer(ctx context.Context, oldId, newId string) (string, error) {
	// currently not do anything to old lb
	t.logger.Infof("Migrate load balancer from %s to %s...", oldId, newId)
	return t.ensureExistLoadBalancer(ctx, newId, nil)
}

// ensure tag, package, ...
func (t *defaultModelDeployTask) ensureExistLoadBalancer(ctx context.Context, lbId string, lbEntity *entity.GlobalLoadBalancer) (string, error) {
	if lbId == "" {
		return "", errors.New("load balancer id is empty")
	}
	if lbEntity == nil {
		var err error
		lbEntity, err = t.vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbId)
		if err != nil {
			return "", err
		}
	}

	// update status
	if err := t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		obj.Status.LoadBalancerId = &lbId
		// TODO: multi address
		// obj.Status.Address = &lbEntity.Domains
	}); err != nil {
		return "", err
	}

	if err := t.deployTags(ctx, lbId); err != nil {
		return "", err
	}

	var err error
	if lbEntity, err = t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
		return "", err
	}

	if err := t.deployPackageId(ctx, lbEntity); err != nil {
		return "", err
	}

	return lbId, t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		obj.Status.LoadBalancerId = &lbId
		// TODO: multi address
		// obj.Status.Address = &lbEntity.Address
	})
}

// resize load balancer if packageID in spec is different from current one
func (t *defaultModelDeployTask) deployPackageId(ctx context.Context, lbEntity *entity.GlobalLoadBalancer) error {
	if t.lbConfig.Spec.PackageId == nil || *t.lbConfig.Spec.PackageId == "" {
		return nil
	}
	if lbEntity == nil {
		return errors.New("load balancer entity is nil")
	}
	if *t.lbConfig.Spec.PackageId == lbEntity.Package {
		return nil
	}

	t.logger.Infof("Need resize loadbalancer from package %s -> %s", lbEntity.Package, *t.lbConfig.Spec.PackageId)
	// TODO: check if can resize
	if err := t.vngcloudRepo.ResizeLoadBalancer(ctx, lbEntity.ID, *t.lbConfig.Spec.PackageId); err != nil {
		return err
	}
	if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbEntity.ID); err != nil {
		t.logger.Error("Failed to wait for loadbalancer active: ", err)
		return err
	}
	return nil
}

func (t *defaultModelDeployTask) buildCreateLoadBalancerRequest(ctx context.Context) (global.ICreateGlobalLoadBalancerRequest, error) {
	packageId, err := t.buildPackageId(ctx)
	if err != nil {
		return nil, err
	}

	request := global.NewCreateGlobalLoadBalancerRequest(
		t.lbConfig.Spec.Name,
	).WithPackage(packageId).
		WithType(global.GlobalLoadBalancerTypeLayer4).
		WithPaymentFlow(global.GlobalLoadBalancerPaymentFlowAutomated)

	if t.lbConfig.Spec.Description != nil {
		request = request.WithDescription(*t.lbConfig.Spec.Description)
	}

	// if have pool, create first pool and listener
	defaultPoolRequest, defaultListenerRequest, err := func() (global.ICreateGlobalPoolRequest, global.ICreateGlobalListenerRequest, error) {
		if len(t.lbConfig.Spec.GlobalPools) == 0 {
			return nil, nil, nil
		}
		poolSpec := t.lbConfig.Spec.GlobalPools[0]

		// Both listener and pool properties must be required (non null) or both are not required (null); <nil> map[]},
		// if have listener, create first listener
		if len(t.lbConfig.Spec.GlobalListeners) == 0 {
			return nil, nil, nil
		}
		listenerSpec := t.lbConfig.Spec.GlobalListeners[0]
		listenerRequest, err := t.buildCreateListenerRequest(ctx, "", listenerSpec, []v1alpha1.CreatedGlobalPool{})
		if err != nil {
			return nil, nil, err
		}

		poolRequest := t.buildCreatePoolRequest(ctx, "", &poolSpec)

		return poolRequest, listenerRequest, nil
	}()
	if err != nil {
		return nil, err
	}

	if defaultPoolRequest != nil && defaultListenerRequest != nil {
		request = request.WithGlobalListener(defaultListenerRequest).WithGlobalPool(defaultPoolRequest)
	}

	// TODO: sdk not support add tags when create load balancer
	// // if have tags, add tags
	// ensuredTags := make(map[string]string)
	// if t.lbConfig.Spec.Tags != nil {
	// 	ensuredTags = t.lbConfig.Spec.Tags
	// }

	// _, newTags := t.buildTag(ctx, map[string]string{}, map[string]string{}, ensuredTags)
	// if len(newTags) > 0 {
	// 	tags := make([]string, 0)
	// 	for k, v := range newTags {
	// 		tags = append(tags, k, v)
	// 	}
	// 	request = request.WithTags(tags...)
	// }

	return request, nil
}

func (t *defaultModelDeployTask) buildPackageId(ctx context.Context) (string, error) {
	// TODO: implement if needed
	return "", nil
}
