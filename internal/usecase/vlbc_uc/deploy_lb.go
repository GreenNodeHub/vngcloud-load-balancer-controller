package vlbc_uc

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
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
	vngcloudRepo repository.IVngCloudRepository
	k8sRepo      repository.IK8sRepository
	vlbConfig    *v1alpha1.VngcloudLoadBalancerConfig
}

func (t *defaultModelDeployTask) deploy(ctx context.Context) error {
	lbId, err := t.deployLoadBalancer(ctx)
	if err != nil {
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

	err = t.deployDeleteRedundantListeners(ctx, lbId, mapListenerPortToID, t.vlbConfig.Status)
	if err != nil {
		return err
	}
	err = t.deployDeleteRedundantPools(ctx, lbId, t.vlbConfig.Status)
	if err != nil {
		return err
	}

	// update status
	return t.k8sRepo.PatchMutateStatusVLBC(ctx, t.vlbConfig, func(ctx context.Context, obj *v1alpha1.VngcloudLoadBalancerConfig) {
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
	if t.vlbConfig.Status.LoadBalancerId != nil && *t.vlbConfig.Status.LoadBalancerId != "" {
		if t.vlbConfig.Spec.LoadBalancerId != nil && *t.vlbConfig.Spec.LoadBalancerId != "" {
			return t.migrateLoadBalancer(ctx, *t.vlbConfig.Status.LoadBalancerId, *t.vlbConfig.Spec.LoadBalancerId)
		} else {
			return t.ensureExistLoadBalancer(ctx, *t.vlbConfig.Status.LoadBalancerId, nil)
		}
	}

	// if not exist lbId in status
	// try to use spec lbId
	if t.vlbConfig.Spec.LoadBalancerId != nil && *t.vlbConfig.Spec.LoadBalancerId != "" {
		return t.ensureExistLoadBalancer(ctx, *t.vlbConfig.Spec.LoadBalancerId, nil)
	}

	// try to use spec lb Name
	if t.vlbConfig.Spec.LoadBalancerName != "" {
		lb, err := t.vngcloudRepo.GetLoadBalancerByName(ctx, t.vlbConfig.Spec.LoadBalancerName)
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
	lbEntity, err := t.vngcloudRepo.CreateLoadBalancer(ctx, t.buildCreateLoadBalancerRequest(ctx))
	if err != nil {
		return "", err
	}
	if lbEntity == nil || lbEntity.UUID == "" {
		return "", errors.New("load balancer not have UUID after create, need to retry")
	}

	// wait for loadbalancer active, if lb is error, delete it and return error
	if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbEntity.UUID); err != nil {
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
	if err := t.k8sRepo.PatchMutateStatusVLBC(ctx, t.vlbConfig, func(ctx context.Context, obj *v1alpha1.VngcloudLoadBalancerConfig) {
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

	return lbId, t.k8sRepo.PatchMutateStatusVLBC(ctx, t.vlbConfig, func(ctx context.Context, obj *v1alpha1.VngcloudLoadBalancerConfig) {
		obj.Status.LoadBalancerId = &lbId
		obj.Status.Address = &lbEntity.Address
	})
}

// resize load balancer if packageID in spec is different from current one
// ignore if this lb is autoscaled
func (t *defaultModelDeployTask) deployPackageId(ctx context.Context, lbEntity *entity.LoadBalancer) error {
	if t.vlbConfig.Spec.PackageID == nil || *t.vlbConfig.Spec.PackageID == "" {
		return nil
	}
	if lbEntity == nil {
		return errors.New("load balancer entity is nil")
	}
	if *t.vlbConfig.Spec.PackageID == lbEntity.PackageID {
		return nil
	}

	if lbEntity.AutoScalable {
		t.logger.Infof("Loadbalancer is autoscaled, skip resizing.")
		return nil
	}

	t.logger.Infof("Need resize loadbalancer from package %s -> %s", lbEntity.PackageID, *t.vlbConfig.Spec.PackageID)
	if err := t.vngcloudRepo.ResizeLoadBalancer(ctx, lbEntity.UUID, *t.vlbConfig.Spec.PackageID); err != nil {
		return err
	}
	if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbEntity.UUID); err != nil {
		t.logger.Error("Failed to wait for loadbalancer active: ", err)
		return err
	}
	return nil
}

func (t *defaultModelDeployTask) buildCreateLoadBalancerRequest(ctx context.Context) loadbalancerv2.ICreateLoadBalancerRequest {
	request := loadbalancerv2.NewCreateLoadBalancerRequest(
		t.vlbConfig.Spec.LoadBalancerName,
		t.cfg.LoadBalancerOpts.DefaultL4PackageId,
		t.vlbConfig.Spec.SubnetID,
	)

	if t.vlbConfig.Spec.PackageID != nil && *t.vlbConfig.Spec.PackageID != "" {
		request = request.WithPackageId(*t.vlbConfig.Spec.PackageID)
	}

	if t.vlbConfig.Spec.Scheme != nil {
		request = request.WithScheme(*t.vlbConfig.Spec.Scheme)
	}

	if t.vlbConfig.Spec.EnableAutoscale != nil {
		request = request.WithAutoScalable(*t.vlbConfig.Spec.EnableAutoscale)
	}

	if t.vlbConfig.Spec.Type != "" {
		request = request.WithType(t.vlbConfig.Spec.Type)
	}

	if t.vlbConfig.Spec.IsPoc != nil {
		request = request.WithPoc(*t.vlbConfig.Spec.IsPoc)
	}

	if t.vlbConfig.Spec.ZoneId != nil {
		request = request.WithZoneId(*t.vlbConfig.Spec.ZoneId)
	}

	// TODO: add more fields
	// WithListener(plistener ICreateListenerRequest) ICreateLoadBalancerRequest
	// WithPool(ppool ICreatePoolRequest) ICreateLoadBalancerRequest
	// WithTags(ptags ...string) ICreateLoadBalancerRequest

	return request
}

// func  (t *defaultModelDeployTask)
