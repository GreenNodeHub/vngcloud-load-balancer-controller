package lbc_uc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/inter"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

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
	lbConfig     *v1alpha1.LoadBalancerConfig
}

func (t *defaultModelDeployTask) deploy(ctx context.Context) error {
	// LAYER 1: Validate the LoadBalancerConfig itself (internal consistency)
	// This runs before we even get the load balancer ID
	if err := t.validateSelf(ctx); err != nil {
		return err
	}

	lbId, err := t.deployLoadBalancer(ctx)
	if err != nil {
		return err
	}

	// LAYER 2: Validate across LoadBalancerConfigs sharing the same load balancer
	// This runs after we have the load balancer ID
	if err := t.validateCrossLBCs(ctx, lbId); err != nil {
		return err
	}

	mapPoolNameToID, err := t.deployPools(ctx, lbId)
	if err != nil {
		return err
	}
	mapListenerPortToID, err := t.deployListeners(ctx, lbId, mapPoolNameToID)
	if err != nil {
		return err
	}

	err = t.deployDeleteRedundantListeners(ctx, lbId, mapListenerPortToID, t.lbConfig.Status)
	if err != nil {
		return err
	}
	err = t.deployDeleteRedundantPools(ctx, lbId, t.lbConfig.Status)
	if err != nil {
		return err
	}

	// update status
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.CreatedListeners = make([]v1alpha1.CreatedListener, 0)
		for _, listenerId := range mapListenerPortToID {
			obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedListener{
				Id: listenerId,
			})
		}
		obj.Status.CreatedPools = make([]v1alpha1.CreatedPool, 0)
		for _, poolId := range mapPoolNameToID {
			obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedPool{
				Id: poolId,
			})
		}
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
	if t.lbConfig.Spec.LoadBalancerName != "" {
		lb, err := t.vngcloudRepo.GetLoadBalancerByName(ctx, t.lbConfig.Spec.LoadBalancerName)
		if err != nil {
			return "", err
		}
		if lb != nil {
			return t.ensureExistLoadBalancer(ctx, lb.UUID, lb)
		}
	}

	return t.createLoadBalancer(ctx)
}

func (t *defaultModelDeployTask) createLoadBalancer(ctx context.Context) (string, error) {
	var lbEntity *entity.LoadBalancer
	var err error
	// check if lb scheme is intervpc, need super client
	if t.lbConfig.Spec.Scheme != nil && *t.lbConfig.Spec.Scheme == loadbalancerv2.InterVpcLoadBalancerScheme {
		createRequest, err := t.buildCreateInterVpcLoadBalancerRequest(ctx)
		if err != nil {
			return "", err
		}
		lbEntity, err = t.vngcloudRepo.CreateInterLoadBalancer(ctx, createRequest)
		if err != nil {
			return "", err
		}
		if lbEntity == nil || lbEntity.UUID == "" {
			return "", errors.New("load balancer not have UUID after create, need to retry")
		}
	} else {
		createRequest, err := t.buildCreateLoadBalancerRequest(ctx)
		if err != nil {
			return "", err
		}
		lbEntity, err = t.vngcloudRepo.CreateLoadBalancer(ctx, createRequest)
		if err != nil {
			return "", err
		}
		if lbEntity == nil || lbEntity.UUID == "" {
			return "", errors.New("load balancer not have UUID after create, need to retry")
		}
	}

	// wait for loadbalancer active, if lb is error, delete it and return error
	if _, err = t.vngcloudRepo.WaitForLBActive(ctx, lbEntity.UUID); err != nil {
		if err == domain.ErrorLoadBalancerStatusError {
			if err := t.vngcloudRepo.DeleteLoadBalancer(ctx, lbEntity.UUID); err != nil {
				t.logger.Error("Failed to delete loadbalancer: ", err)
				return "", err
			}
			t.logger.Infof("Delete loadbalancer \"%s\" because of status error, recreate now.", lbEntity.UUID)
			return "", errs.NewRequeueNeeded("loadbalancer status is error, delete and recreate")
		}
		t.logger.Error("Failed to wait for loadbalancer active: ", err)
		return "", err
	}

	t.logger.Infof("Created load balancer with ID %s", lbEntity.UUID)
	return t.ensureExistLoadBalancer(ctx, lbEntity.UUID, nil)
}

// when update load balancer id to new value
func (t *defaultModelDeployTask) migrateLoadBalancer(ctx context.Context, oldId, newId string) (string, error) {
	// currently not do anything to old lb
	t.logger.Infof("Migrate load balancer from %s to %s...", oldId, newId)
	return t.ensureExistLoadBalancer(ctx, newId, nil)
}

// ensure tag, package, ...
func (t *defaultModelDeployTask) ensureExistLoadBalancer(ctx context.Context, lbId string, lbEntity *entity.LoadBalancer) (string, error) {
	if lbId == "" {
		return "", errors.New("load balancer id is empty")
	}
	if lbEntity == nil {
		var err error
		lbEntity, err = t.vngcloudRepo.GetLoadBalancerByID(ctx, lbId)
		if err != nil {
			return "", err
		}
	}

	// update status
	if err := t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.LoadBalancerId = &lbId
		obj.Status.Address = &lbEntity.Address
	}); err != nil {
		return "", err
	}

	if err := t.deployTags(ctx, lbId); err != nil {
		return "", err
	}

	if err := t.deployPackageId(ctx, lbEntity); err != nil {
		return "", err
	}

	return lbId, t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.LoadBalancerId = &lbId
		obj.Status.Address = &lbEntity.Address
	})
}

// resize load balancer if packageID in spec is different from current one
// ignore if this lb is autoscaled
func (t *defaultModelDeployTask) deployPackageId(ctx context.Context, lbEntity *entity.LoadBalancer) error {
	if t.lbConfig.Spec.PackageId == nil || *t.lbConfig.Spec.PackageId == "" {
		return nil
	}
	if lbEntity == nil {
		return errors.New("load balancer entity is nil")
	}
	if *t.lbConfig.Spec.PackageId == lbEntity.PackageID {
		return nil
	}

	if lbEntity.AutoScalable {
		t.logger.Infof("Loadbalancer is autoscaled, skip resizing.")
		return nil
	}

	t.logger.Infof("Need resize loadbalancer from package %s -> %s", lbEntity.PackageID, *t.lbConfig.Spec.PackageId)
	if err := t.vngcloudRepo.ResizeLoadBalancer(ctx, lbEntity.UUID, *t.lbConfig.Spec.PackageId); err != nil {
		return err
	}
	if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbEntity.UUID); err != nil {
		t.logger.Error("Failed to wait for loadbalancer active: ", err)
		return err
	}
	return nil
}

func (t *defaultModelDeployTask) buildCreateLoadBalancerRequest(ctx context.Context) (loadbalancerv2.ICreateLoadBalancerRequest, error) {
	packageId := ""
	if t.lbConfig.Spec.PackageId != nil && *t.lbConfig.Spec.PackageId != "" {
		packageId = *t.lbConfig.Spec.PackageId
	} else {
		// use default package from config, get default package id from name and zone
		listPackages, err := t.vngcloudRepo.ListLoadBalancerPackageByZone(ctx, t.lbConfig.Spec.ZoneId)
		if err != nil {
			return nil, err
		}
		for _, pkg := range listPackages.Items {
			if pkg.Name == t.cfg.LoadBalancerOpts.DefaultL4PackageName {
				packageId = pkg.UUID
				break
			}
		}
		if packageId == "" {
			return nil, errs.NewNoNeedRequeue(fmt.Sprintf("cannot find default load balancer package %s in zone %s", t.cfg.LoadBalancerOpts.DefaultL4PackageName, t.lbConfig.Spec.ZoneId))
		}
	}

	request := loadbalancerv2.NewCreateLoadBalancerRequest(
		t.lbConfig.Spec.LoadBalancerName,
		packageId,
		t.lbConfig.Spec.SubnetId,
	).WithZoneId(t.lbConfig.Spec.ZoneId).WithScheme(loadbalancerv2.LoadBalancerScheme(t.cfg.LoadBalancerOpts.DefaultScheme))

	if t.lbConfig.Spec.Scheme != nil {
		request = request.WithScheme(*t.lbConfig.Spec.Scheme)
	}

	if t.lbConfig.Spec.EnableAutoscale != nil {
		request = request.WithAutoScalable(*t.lbConfig.Spec.EnableAutoscale)
	}

	if t.lbConfig.Spec.Type != "" {
		request = request.WithType(t.lbConfig.Spec.Type)
	}

	if t.lbConfig.Spec.IsPoc != nil {
		request = request.WithPoc(*t.lbConfig.Spec.IsPoc)
	}

	// TODO: add more fields
	// WithListener(plistener ICreateListenerRequest) ICreateLoadBalancerRequest
	// WithPool(ppool ICreatePoolRequest) ICreateLoadBalancerRequest
	// WithTags(ptags ...string) ICreateLoadBalancerRequest

	return request, nil
}

func (t *defaultModelDeployTask) buildCreateInterVpcLoadBalancerRequest(ctx context.Context) (inter.ICreateLoadBalancerRequest, error) {
	// check if userId is provided
	userId := t.cfg.Global.UserID
	if userId == 0 {
		// try to get from vngcloudRepo
		if userId = t.vngcloudRepo.GetUserId(); userId == 0 {
			return nil, errs.NewNoNeedRequeue("userId is required, cannot get from config or vngcloud repository")
		}
	}

	// build backendSubnetId
	if t.lbConfig.Spec.BackendSubnetId == nil || *t.lbConfig.Spec.BackendSubnetId == "" {
		return nil, errs.NewNoNeedRequeue("backendSubnetId is required for InterVpc load balancer")
	}

	// build packageId
	packageId := ""
	if t.lbConfig.Spec.PackageId != nil && *t.lbConfig.Spec.PackageId != "" {
		packageId = *t.lbConfig.Spec.PackageId
	} else {
		// use default package from config, get default package id from name and zone
		listPackages, err := t.vngcloudRepo.ListLoadBalancerPackageByZone(ctx, t.lbConfig.Spec.ZoneId)
		if err != nil {
			return nil, err
		}
		for _, pkg := range listPackages.Items {
			if pkg.Name == t.cfg.LoadBalancerOpts.DefaultL4PackageName {
				packageId = pkg.UUID
				break
			}
		}
		if packageId == "" {
			return nil, errs.NewNoNeedRequeue(fmt.Sprintf("cannot find default load balancer package %s in zone %s", t.cfg.LoadBalancerOpts.DefaultL4PackageName, t.lbConfig.Spec.ZoneId))
		}
	}

	request := inter.NewCreateLoadBalancerRequest(
		strconv.Itoa(userId),
		t.lbConfig.Spec.LoadBalancerName,
		packageId,
		t.lbConfig.Spec.SubnetId,
		*t.lbConfig.Spec.BackendSubnetId,
	).WithZoneId(t.lbConfig.Spec.ZoneId)

	// if t.lbConfig.Spec.Scheme != nil {
	// 	request = request.WithScheme(*t.lbConfig.Spec.Scheme)
	// }

	// if t.lbConfig.Spec.EnableAutoscale != nil {
	// 	request = request.WithAutoScalable(*t.lbConfig.Spec.EnableAutoscale)
	// }

	// if t.lbConfig.Spec.Type != "" {
	// 	request = request.WithType(t.lbConfig.Spec.Type)
	// }

	// if t.lbConfig.Spec.IsPoc != nil {
	// 	request = request.WithPoc(*t.lbConfig.Spec.IsPoc)
	// }

	// TODO: add more fields
	// WithListener(plistener ICreateListenerRequest) ICreateLoadBalancerRequest
	// WithPool(ppool ICreatePoolRequest) ICreateLoadBalancerRequest
	// WithTags(ptags ...string) ICreateLoadBalancerRequest

	return request, nil
}
