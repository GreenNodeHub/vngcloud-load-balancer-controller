package lbc_uc

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

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
	if _, err := t.vngcloudRepo.GetLoadBalancerByID(ctx, lbId); err != nil {
		if domain.IsLoadBalancerNotFound(err) {
			return nil
		}
		return err
	}

	// Only a load balancer this cluster created is ours to delete. Reading the tags is how
	// provenance is established, and the read is cached - deployTags has just made it.
	tags, err := t.vngcloudRepo.ListTags(ctx, lbId)
	if err != nil {
		return err
	}
	currentTags := make(map[string]string, len(tags.Items))
	for _, tag := range tags.Items {
		currentTags[tag.Key] = tag.Value
	}
	ours := t.createdByThisCluster(lbId, currentTags)
	if !ours {
		t.logger.Infof("Load balancer %s was not created by this cluster, it will be left in place for LBC %s/%s",
			lbId, t.lbConfig.Namespace, t.lbConfig.Name)
	}

	canDelete, err := t.canDeleteWholeLoadBalancer(ctx, lbId, t.lbConfig.Spec.Type)
	if err != nil {
		return err
	}
	if canDelete && ours {
		t.logger.Infof("Deleting load balancer %s in VNGCloud for LBC %s/%s", lbId, t.lbConfig.Namespace, t.lbConfig.Name)
		err = t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)
		if err != nil {
			return err
		}
	} else {
		// delete redundant listeners
		if err := t.deleteRedundantListeners(ctx, lbId, []v1alpha1.CreatedListener{}, []v1alpha1.CreatedPool{}); err != nil {
			return err
		}

		// delete redundant pools, should check if pool is used by other listeners or policy then ignore
		if err := t.deleteRedundantPools(ctx, lbId, []v1alpha1.CreatedPool{}); err != nil {
			return err
		}

		// after deleting lis and pools, check if currentBuilder is empty, if so, delete loadbalancer
		// this is because CanDeleteWholeLoadBalancer sometimes return false because status is not updated yet but user has deleted svcLB
		isEmpty, err := t.isLoadBalancerEmpty(ctx, lbId)
		if err != nil {
			return err
		}
		if isEmpty && ours {
			t.logger.Infof("Load balancer %s is empty, deleting it in VNGCloud for LBC %s/%s", lbId, t.lbConfig.Namespace, t.lbConfig.Name)
			err = t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)
			if err != nil {
				return err
			}
		} else {
			// load balancer still exists, remove cluster id from cluster tags
			if err := t.deleteRedundantTags(ctx, lbId); err != nil {
				return err
			}
		}
	}
	return nil
}

// check if can delete whole loadbalancer
// oldBuilder and currentBuilder should be the same listeners' name, pool's name
// if can delete whole loadbalancer, delete loadbalancer and return
func (t *defaultModelDeployTask) canDeleteWholeLoadBalancer(ctx context.Context, lbId string, lbType loadbalancerv2.LoadBalancerType) (bool, error) {
	// get current listeners from vngcloud
	listeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return false, err
	}

	// all listeners must be in .status.createdListeners
	_canCoverListener, err := canCover(t.lbConfig.Status.CreatedListeners, listeners.Items, func(a []v1alpha1.CreatedListener, b *entityv2.Listener) (bool, error) {
		found := false
		createdListener := v1alpha1.CreatedListener{}
		for _, oldL := range a {
			if oldL.Id == b.UUID {
				found = true
				createdListener = oldL
				break
			}
		}
		if !found {
			t.logger.Debugf("Cannot delete whole loadbalancer because listener %s is not in status", b.Name)
			return false, nil
		}

		// skip policy check for L4 load balancer (L4 has no policies)
		if lbType == loadbalancerv2.LoadBalancerTypeLayer4 {
			return true, nil
		}

		// check policies, all policies must be in .status.createdPolicies
		currentPolicies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, createdListener.Id)
		if err != nil {
			return false, err
		}
		_canCoverPolicies, _ := canCover(createdListener.CreatedPolicies, currentPolicies.Items, func(a []v1alpha1.CreatedPolicy, b *entityv2.Policy) (bool, error) {
			foundPolicy := false
			for _, oldP := range a {
				if oldP.Id == b.UUID {
					foundPolicy = true
					break
				}
			}
			if !foundPolicy {
				t.logger.Debugf("Cannot delete whole loadbalancer because policy %s is not in status", b.Name)
				return false, nil
			}
			return true, nil
		})
		return _canCoverPolicies, nil
	})
	if err != nil {
		return false, err
	}
	if !_canCoverListener {
		return false, nil
	}

	pools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		return false, err
	}

	// check pools, all pools must be in .status.createdPools
	_canCoverPool, err := canCover(t.lbConfig.Status.CreatedPools, pools.Items, func(a []v1alpha1.CreatedPool, b *entityv2.Pool) (bool, error) {
		found := false
		createPool := v1alpha1.CreatedPool{}
		for _, oldP := range a {
			if oldP.Id == b.UUID {
				found = true
				createPool = oldP
				break
			}
		}
		if !found {
			t.logger.Debugf("Cannot delete whole loadbalancer because pool %s is not in status", b.Name)
			return false, nil
		}

		// check members, all members must be in .status.createdMembers
		currentMembers, err := t.vngcloudRepo.GetPoolMembers(ctx, lbId, b.UUID)
		if err != nil {
			return false, err
		}
		_canCoverMembers, _ := canCover(createPool.CreatedMembers, currentMembers.Items, func(a []v1alpha1.PoolMember, b *entityv2.Member) (bool, error) {
			if !t.checkIfPoolMemberExist(a, convertMember(b)) {
				t.logger.Debugf("Cannot delete whole loadbalancer because member %s is not in status", b.Address)
				return false, nil
			}
			return true, nil
		})
		return _canCoverMembers, nil
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
