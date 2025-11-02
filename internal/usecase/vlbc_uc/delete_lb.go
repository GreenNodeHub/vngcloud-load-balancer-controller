package vlbc_uc

import (
	"context"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

type defaultModelDeleteTask struct {
	logger       *logrus.Entry
	cfg          *config.Config
	vngcloudRepo repository.IVngCloudRepository
	k8sRepo      repository.IK8sRepository
	vlbConfig    *v1alpha1.VngcloudLoadBalancerConfig
}

func (t *defaultModelDeleteTask) delete(ctx context.Context) error {
	// if status.lbId is empty, skip
	if t.vlbConfig.Status.LoadBalancerId == nil || *t.vlbConfig.Status.LoadBalancerId == "" {
		t.logger.Infof("VLBC %s/%s has no LoadBalancerId in status, skip deleting load balancer in VNGCloud", t.vlbConfig.Namespace, t.vlbConfig.Name)
		return nil
	}

	lbId := *t.vlbConfig.Status.LoadBalancerId

	if err := t.deleteLoadBalancer(ctx, lbId); err != nil {
		return err
	}

	return nil
}

func (t *defaultModelDeleteTask) deleteLoadBalancer(ctx context.Context, lbId string) error {
	// check if load balancer is exists
	if _, err := t.vngcloudRepo.GetLoadBalancerByID(ctx, lbId); err != nil {
		if domain.IsLoadBalancerNotFound(err) {
			return nil
		}
		return err
	}

	canDelete, err := t.canDeleteWholeLoadBalancer(ctx, lbId)
	if err != nil {
		return err
	}
	if canDelete {
		t.logger.Infof("Deleting load balancer %s in VNGCloud for VLBC %s/%s", lbId, t.vlbConfig.Namespace, t.vlbConfig.Name)
		err = t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)
		if err != nil {
			return err
		}
	} else {
		// delete redundant listeners
		if err := t.deleteRedundantListeners(ctx, lbId); err != nil {
			return err
		}

		// delete redundant pools, should check if pool is used by other listeners or policy then ignore
		if err := t.deleteRedundantPools(ctx, lbId); err != nil {
			return err
		}

		// after deleting lis and pools, check if currentBuilder is empty, if so, delete loadbalancer
		// this is because CanDeleteWholeLoadBalancer sometimes return false because status is not updated yet but user has deleted svcLB
		isEmpty, err := t.isLoadBalancerEmpty(ctx, lbId)
		if err != nil {
			return err
		}
		if isEmpty {
			t.logger.Infof("Load balancer %s is empty, deleting it in VNGCloud for VLBC %s/%s", lbId, t.vlbConfig.Namespace, t.vlbConfig.Name)
			err = t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// check if can delete whole loadbalancer
// oldBuilder and currentBuilder should be the same listeners' name, pool's name
// if can delete whole loadbalancer, delete loadbalancer and return
func (t *defaultModelDeleteTask) canDeleteWholeLoadBalancer(ctx context.Context, lbId string) (bool, error) {
	// get current listeners and pools from vngcloud
	listeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return false, err
	}
	// if len(oldListeners) < len(currentListeners), can't delete whole loadbalancer
	// because some listeners are created by other resources
	if len(t.vlbConfig.Status.CreatedListeners) < len(listeners.Items) {
		t.logger.Infof("Can't delete whole loadbalancer, len(oldListeners) < len(currentListeners) (%d < %d)",
			len(t.vlbConfig.Status.CreatedListeners), len(listeners.Items))
		return false, nil
	}

	pools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		return false, err
	}
	if len(t.vlbConfig.Status.CreatedPools) < len(pools.Items) {
		t.logger.Infof("Can't delete whole loadbalancer, len(oldPools) < len(currentPools) (%d < %d)",
			len(t.vlbConfig.Status.CreatedPools), len(pools.Items))
		return false, nil
	}

	searchListenerById := func(id string) *entityv2.Listener {
		for _, l := range listeners.Items {
			if l.UUID == id {
				return l
			}
		}
		return nil
	}

	// if listener not exists, return false
	// TODO: why? this is old logic, need to confirm
	for _, listener := range t.vlbConfig.Status.CreatedListeners {
		currentListener := searchListenerById(listener.Id)
		if currentListener == nil {
			t.logger.Infof("Can't delete whole loadbalancer, listener not exists: %s", listener.Id)
			return false, nil
		}

		// TODO: uncomment and fix this part
		// // if policy not exists, return false
		// currentPolicies := currentListener.GetPolicyBuilders()
		// oldPolicies := listener.GetOldPolicies()
		// if len(oldPolicies) < len(currentPolicies) {
		// 	r.logger.Infof("Can't delete whole loadbalancer, len(oldPolicies) < len(currentPolicies) (%d < %d)",
		// 		len(oldPolicies), len(currentPolicies))
		// 	return false
		// }
		// for _, policy := range oldPolicies {
		// 	if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy == nil {
		// 		r.logger.Infof("Can't delete whole loadbalancer, policy not exists: %s", policy.GetName())
		// 		return false
		// 	}
		// }
	}

	searchPoolById := func(id string) *entityv2.Pool {
		for _, p := range pools.Items {
			if p.UUID == id {
				return p
			}
		}
		return nil
	}

	// if pool not exists, return false
	for _, pool := range t.vlbConfig.Status.CreatedPools {
		if currentPool := searchPoolById(pool.Id); currentPool == nil {
			t.logger.Infof("Can't delete whole loadbalancer, pool not exists: %s", pool.Id)
			return false, nil
		}
	}

	// TODO: uncomment me
	// // if default pool members not match, return false
	// if defaultPool := r.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL); defaultPool != nil {
	// 	currentMembers := defaultPool.Members
	// 	oldMembers := oldBuilder.GetDefaultPoolMembers()
	// 	if len(oldMembers) < len(currentMembers) {
	// 		r.logger.Infof("Can't delete whole loadbalancer, len(oldDFMembers) < len(currentDFMembers) (%d < %d)",
	// 			len(oldMembers), len(currentMembers))
	// 		return false
	// 	}
	// 	if !r.comparePoolMembers(oldMembers, currentMembers, true) {
	// 		t.logger.Infof("Can't delete whole loadbalancer, default pool members not match")
	// 		return false
	// 	}
	// }

	t.logger.Debug("Can delete whole loadbalancer")
	return true, nil
}

func (t *defaultModelDeleteTask) isLoadBalancerEmpty(ctx context.Context, lbId string) (bool, error) {
	listeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return false, err
	}
	if len(listeners.Items) > 0 {
		return false, nil
	}

	pools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		return false, err
	}
	if len(pools.Items) > 0 {
		return false, nil
	}

	return true, nil
}
