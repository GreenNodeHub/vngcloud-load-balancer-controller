package glbc_uc

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

func (t *defaultModelDeployTask) delete(ctx context.Context) error {
	// if status.lbId is empty, skip
	if t.lbConfig.Status.LoadBalancerId == nil || *t.lbConfig.Status.LoadBalancerId == "" {
		t.logger.Infof("LBC %s/%s has no LoadBalancerId in status, skip deleting load balancer in VNGCloud", t.lbConfig.Namespace, t.lbConfig.Name)
		return nil
	}

	lbId := *t.lbConfig.Status.LoadBalancerId

	if err := t.deleteLoadBalancer(ctx, lbId); err != nil {
		return err
	}

	return nil
}

func (t *defaultModelDeployTask) deleteLoadBalancer(ctx context.Context, lbId string) error {
	// check if load balancer is exists
	if _, err := t.vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbId); err != nil {
		if domain.IsGlobalLoadBalancerNotFound(err) {
			return nil
		}
		return err
	}

	canDelete, err := t.canDeleteWholeLoadBalancer(ctx, lbId)
	if err != nil {
		return err
	}
	if canDelete {
		t.logger.Infof("Deleting load balancer %s in VNGCloud for LBC %s/%s", lbId, t.lbConfig.Namespace, t.lbConfig.Name)
		err = t.vngcloudRepo.DeleteGlobalLoadBalancer(ctx, lbId)
		if err != nil {
			return err
		}
	} else {
		// delete redundant listeners
		if err := t.deleteRedundantListeners(ctx, lbId, []v1alpha1.CreatedGlobalListener{}, []v1alpha1.CreatedGlobalPool{}); err != nil {
			return err
		}

		// delete redundant pools, should check if pool is used by other listeners or policy then ignore
		if err := t.deleteRedundantPools(ctx, lbId, []v1alpha1.CreatedGlobalPool{}); err != nil {
			return err
		}

		// after deleting lis and pools, check if currentBuilder is empty, if so, delete loadbalancer
		// this is because CanDeleteWholeLoadBalancer sometimes return false because status is not updated yet but user has deleted svcLB
		isEmpty, err := t.isLoadBalancerEmpty(ctx, lbId)
		if err != nil {
			return err
		}
		if isEmpty {
			t.logger.Infof("Load balancer %s is empty, deleting it in VNGCloud for LBC %s/%s", lbId, t.lbConfig.Namespace, t.lbConfig.Name)
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
func (t *defaultModelDeployTask) canDeleteWholeLoadBalancer(ctx context.Context, lbId string) (bool, error) {
	// get current listeners from vngcloud
	listeners, err := t.vngcloudRepo.ListGlobalListeners(ctx, lbId)
	if err != nil {
		return false, err
	}

	// all listeners must be in .status.createdListeners
	_canCoverListener, err := canCover(t.lbConfig.Status.CreatedListeners, listeners.Items, func(a []v1alpha1.CreatedGlobalListener, b *entityv2.GlobalListener) (bool, error) {
		found := false
		for _, oldL := range a {
			if oldL.Id == b.ID {
				found = true
				break
			}
		}
		if !found {
			t.logger.Debugf("Cannot delete whole loadbalancer because listener %s is not in status", b.Name)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return false, err
	}
	if !_canCoverListener {
		return false, nil
	}

	pools, err := t.vngcloudRepo.ListGlobalPools(ctx, lbId)
	if err != nil {
		return false, err
	}

	// check pools, all pools must be in .status.createdPools
	_canCoverPool, err := canCover(t.lbConfig.Status.CreatedPools, pools.Items, func(a []v1alpha1.CreatedGlobalPool, b *entityv2.GlobalPool) (bool, error) {
		found := false
		// createPool := v1alpha1.CreatedGlobalPool{}
		for _, oldP := range a {
			if oldP.Id == b.ID {
				found = true
				// createPool = oldP
				break
			}
		}
		if !found {
			t.logger.Debugf("Cannot delete whole loadbalancer because pool %s is not in status", b.Name)
			return false, nil
		}
		return true, nil

		// TODO
		// // check members, all members must be in .status.createdMembers
		// currentMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, b.ID)
		// if err != nil {
		// 	return false, err
		// }
		// _canCoverMembers, err := canCover(createPool.CreatedMembers, currentMembers.Items, func(a []v1alpha1.PoolMember, b *entityv2.Member) (bool, error) {
		// 	if !t.checkIfPoolMemberExist(a, convertMember(b)) {
		// 		t.logger.Debugf("Cannot delete whole loadbalancer because member %s is not in status", b.Address)
		// 		return false, nil
		// 	}
		// 	return true, nil
		// })
		// if err != nil {
		// 	return false, err
		// }
		// return _canCoverMembers, nil
	})
	if err != nil {
		return false, err
	}
	if !_canCoverPool {
		return false, nil
	}

	t.logger.Debug("Can delete whole loadbalancer")
	return true, nil
}

func (t *defaultModelDeployTask) isLoadBalancerEmpty(ctx context.Context, lbId string) (bool, error) {
	listeners, err := t.vngcloudRepo.ListGlobalListeners(ctx, lbId)
	if err != nil {
		return false, err
	}
	if len(listeners.Items) > 0 {
		return false, nil
	}

	pools, err := t.vngcloudRepo.ListGlobalPools(ctx, lbId)
	if err != nil {
		return false, err
	}
	if len(pools.Items) > 0 {
		return false, nil
	}

	return true, nil
}

// canCover checks if all elements in smallOne are present in bigOne using the isExist function.
func canCover[T, U any](bigOne []T, smallOne []U, isExist func([]T, U) (bool, error)) (bool, error) {
	for _, b := range smallOne {
		exist, err := isExist(bigOne, b)
		if err != nil {
			return false, err
		}
		if !exist {
			return false, nil
		}
	}
	return true, nil
}
