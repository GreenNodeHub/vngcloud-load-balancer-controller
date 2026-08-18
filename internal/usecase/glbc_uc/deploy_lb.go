package glbc_uc

import (
	"context"
	"fmt"

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

// extractAddressInfo extracts VIPs and domains from the GlobalLoadBalancer entity
func extractAddressInfo(lbEntity *entity.GlobalLoadBalancer) (vips []v1alpha1.GlobalLoadBalancerVIPStatus, domains []string) {
	if lbEntity == nil {
		return nil, nil
	}

	// Extract VIPs
	for _, vip := range lbEntity.Vips {
		if vip != nil {
			vips = append(vips, v1alpha1.GlobalLoadBalancerVIPStatus{
				Address: vip.Address,
				Region:  vip.Region,
				Status:  vip.Status,
			})
		}
	}

	// Extract Domains
	for _, d := range lbEntity.Domains {
		if d != nil && d.Hostname != "" {
			domains = append(domains, d.Hostname)
		}
	}

	return vips, domains
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
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) bool {
		// check on fresh copy if already equal
		if createdGlobalListenersEqual(obj.Status.CreatedListeners, newCreatedListeners) &&
			createdGlobalPoolsEqual(obj.Status.CreatedPools, newCreatedPools) {
			return false // no change needed
		}
		obj.Status.CreatedListeners = newCreatedListeners
		obj.Status.CreatedPools = newCreatedPools
		return true
	})
}

// deployLoadBalancer creates or ensures the load balancer exists, handling migrations if necessary.
// It returns the load balancer ID to be used for further operations.
// The caller should requeue if a new load balancer ID is obtained to acquire the appropriate lock.
func (t *defaultModelDeployTask) deployLoadBalancer(ctx context.Context) (string, error) {
	errorNewLbIdObtained := errs.NewRequeueNeeded("new load balancer ID obtained, requeue needed")

	// if already an exist lb
	if t.lbConfig.Status.LoadBalancerId != nil && *t.lbConfig.Status.LoadBalancerId != "" {
		if t.lbConfig.Spec.LoadBalancerId != nil && *t.lbConfig.Spec.LoadBalancerId != "" && *t.lbConfig.Spec.LoadBalancerId != *t.lbConfig.Status.LoadBalancerId {
			lbId, err := t.migrateLoadBalancer(ctx, *t.lbConfig.Status.LoadBalancerId, *t.lbConfig.Spec.LoadBalancerId)
			if err != nil {
				return lbId, err
			}
			return lbId, errorNewLbIdObtained
		} else {
			return t.ensureExistLoadBalancer(ctx, *t.lbConfig.Status.LoadBalancerId, nil)
		}
	}

	// if not exist lbId in status
	// try to use spec lbId
	if t.lbConfig.Spec.LoadBalancerId != nil && *t.lbConfig.Spec.LoadBalancerId != "" {
		lbId, err := t.ensureExistLoadBalancer(ctx, *t.lbConfig.Spec.LoadBalancerId, nil)
		if err != nil {
			return lbId, err
		}
		return lbId, errorNewLbIdObtained
	}

	// try to use spec lb Name
	if t.lbConfig.Spec.Name != "" {
		lb, err := t.vngcloudRepo.GetGlobalLoadBalancerByName(ctx, t.lbConfig.Spec.Name)
		if err != nil {
			return "", err
		}
		if lb != nil {
			lbId, err := t.ensureExistLoadBalancer(ctx, lb.ID, lb)
			if err != nil {
				return lbId, err
			}
			return lbId, errorNewLbIdObtained
		}
	}

	// if requeue here, created Listeners and Pools is not saved in status yet, if user delete LBC quickly, may cause orphan resources
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

	// update status with initial ID (VIPs and Domains may not be available yet)
	vips, domains := extractAddressInfo(lbEntity)
	if err := t.statusAddLoadBalancerId(ctx, &lbEntity.ID, vips, domains); err != nil {
		return "", err
	}

	// wait for loadbalancer active, if lb is error, delete it and return error
	if _, err = t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbEntity.ID); err != nil {
		if err == domain.ErrorLoadBalancerStatusError {
			if err := t.statusAddLoadBalancerId(ctx, nil, nil, nil); err != nil {
				return "", err
			}
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

	// because load balancer is just created, need to deploy package and tag again and update status
	return t.ensureExistLoadBalancer(ctx, lbEntity.ID, nil)
}

// when update load balancer id to new value
func (t *defaultModelDeployTask) migrateLoadBalancer(ctx context.Context, oldId, newId string) (string, error) {
	t.logger.Infof("Migrate load balancer from %s to %s...", oldId, newId)

	// Reclaim the old load balancer before touching the new one: status.createdListeners
	// and status.createdPools are the only record of what we put there, and
	// ensureExistLoadBalancer below overwrites them.
	if err := t.teardownOnLoadBalancer(ctx, oldId); err != nil {
		return "", err
	}

	return t.ensureExistLoadBalancer(ctx, newId, nil)
}

// teardownOnLoadBalancer removes the listeners and pools this GlobalLoadBalancerConfig
// created on lbId, then forgets them in status.
//
// Unlike deleteLoadBalancer it never deletes lbId itself. A load balancer reached
// through spec.loadBalancerId belongs to the user and may still serve other configs, so
// removing it would take down unrelated traffic.
func (t *defaultModelDeployTask) teardownOnLoadBalancer(ctx context.Context, lbId string) error {
	if _, err := t.vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbId); err != nil {
		if domain.IsGlobalLoadBalancerNotFound(err) {
			return nil // already gone, nothing left to reclaim
		}
		return err
	}

	// Listeners first: deleteRedundantPools keeps any pool a listener still points at.
	if err := t.deleteRedundantListeners(ctx, lbId, []v1alpha1.CreatedGlobalListener{}, []v1alpha1.CreatedGlobalPool{}); err != nil {
		return err
	}
	if err := t.deleteRedundantPools(ctx, lbId, []v1alpha1.CreatedGlobalPool{}); err != nil {
		return err
	}

	return t.statusClearCreatedResources(ctx)
}

// ensure tag, package, ...
func (t *defaultModelDeployTask) ensureExistLoadBalancer(ctx context.Context, lbId string, lbEntity *entity.GlobalLoadBalancer) (string, error) {
	if lbId == "" {
		return "", errors.New("load balancer id is empty")
	}
	if lbEntity == nil {
		var err error
		_, err = t.vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbId)
		if err != nil {
			return "", err
		}
	}

	var err error
	if lbEntity, err = t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
		return "", err
	}

	// update status with VIPs and Domains from the active load balancer
	vips, domains := extractAddressInfo(lbEntity)
	if err := t.statusAddLoadBalancerId(ctx, &lbId, vips, domains); err != nil {
		return "", err
	}

	if err := t.deployPackageId(ctx, lbEntity); err != nil {
		return "", err
	}

	return lbId, nil
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
	if t.lbConfig.Spec.PackageId != nil && *t.lbConfig.Spec.PackageId != "" {
		return *t.lbConfig.Spec.PackageId, nil
	}

	// use default package from config, resolve package name to ID via API
	if t.cfg.GlobalLoadBalancerOpts.DefaultL4PackageName != "" {
		listPackages, err := t.vngcloudRepo.ListGlobalPackages(ctx)
		if err != nil {
			return "", err
		}
		for _, pkg := range listPackages.Items {
			if pkg.Name == t.cfg.GlobalLoadBalancerOpts.DefaultL4PackageName {
				return pkg.ID, nil
			}
		}
		return "", errs.NewNoNeedRequeue(fmt.Sprintf("cannot find default global load balancer package %s", t.cfg.GlobalLoadBalancerOpts.DefaultL4PackageName))
	}

	// fallback to hardcoded package ID
	return "pkg-b02e62ab-a282-4faf-8732-a172ef497a7b", nil
}
