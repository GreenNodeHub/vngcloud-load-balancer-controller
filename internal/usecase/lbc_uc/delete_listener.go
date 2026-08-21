package lbc_uc

import (
	"context"
	"errors"
	"fmt"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// delete redundant listeners
// newCreatedListeners is the listeners that are still in use
func (t *defaultModelDeployTask) deleteRedundantListeners(ctx context.Context, lbId string, newCreatedListeners []v1alpha1.CreatedListener, newCreatedPools []v1alpha1.CreatedPool) error {
	return t.deleteRedundantListenersFrom(ctx, lbId, t.lbConfig.Status.CreatedListeners, newCreatedListeners, newCreatedPools)
}

// deleteRedundantListenersFrom is deleteRedundantListeners with the candidate set made
// explicit; see deleteRedundantPoolsFrom for why.
func (t *defaultModelDeployTask) deleteRedundantListenersFrom(ctx context.Context, lbId string, createdListeners []v1alpha1.CreatedListener, newCreatedListeners []v1alpha1.CreatedListener, newCreatedPools []v1alpha1.CreatedPool) error {
	// delete candidates include all created listeners
	deleteCandidates := make([]string, 0)
	for _, listener := range createdListeners {
		deleteCandidates = append(deleteCandidates, listener.Id)
	}

	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return err
	}

	isListenerExist := func(listenerId string) (bool, *entityv2.Listener) {
		for _, l := range currentListeners.Items {
			if l.UUID == listenerId {
				return true, l
			}
		}
		return false, nil
	}

	isListenerInUse := func(listenerId string) bool {
		for _, listener := range newCreatedListeners {
			if listener.Id == listenerId {
				return true
			}
		}
		return false
	}

	// delete redundant listeners
	// Same reasoning as deleteRedundantPools: the candidates are independent, so one that will
	// not go must not stop the others from going. Failures are collected and returned at the
	// end, which still fails the reconcile and retries it.
	failures := make([]error, 0)

	for _, candidateId := range deleteCandidates {
		isExist, listener := isListenerExist(candidateId)
		if !isExist {
			t.logger.Debugf("Listener %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		canDeleteWhole, err := t.canDeleteWholeListener(ctx, lbId, createdListeners, listener, newCreatedPools)
		if err != nil {
			failures = append(failures, fmt.Errorf("listener %s: %w", candidateId, err))
			continue
		}

		if !isListenerInUse(candidateId) && canDeleteWhole {
			err := t.retryOnLoadBalancerNotReady(ctx, lbId, func() error {
				return t.vngcloudRepo.DeleteListener(ctx, lbId, candidateId)
			})
			if err != nil {
				failures = append(failures, fmt.Errorf("listener %s: delete: %w", candidateId, err))
				continue
			}
			if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
				failures = append(failures, fmt.Errorf("listener %s: wait after delete: %w", candidateId, err))
				continue
			}
		} else {
			// delete redundant policy
			newCreatedPolicies := []v1alpha1.CreatedPolicy{}
			for _, l := range newCreatedListeners {
				if l.Id == candidateId {
					newCreatedPolicies = l.CreatedPolicies
					break
				}
			}
			if err := t.deployDeleteRedundantPoliciesFrom(ctx, lbId, createdListeners, candidateId, newCreatedPolicies); err != nil {
				failures = append(failures, fmt.Errorf("listener %s: policies: %w", candidateId, err))
				continue
			}
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%w: %w", errPartialDelete, errors.Join(failures...))
	}
	return nil
}

// canDeleteWholeListener checks if we can delete the whole listener
// conditions:
// - all current policies must exist in old policies
// - if default pool exists, can delete whole pool (case 2 ingress use same listener and default pool (merge their member), when delete one ingress, we should NOT delete whole listener)
func (t *defaultModelDeployTask) canDeleteWholeListener(ctx context.Context, lbId string, createdListeners []v1alpha1.CreatedListener, listener *entityv2.Listener, newCreatedPools []v1alpha1.CreatedPool) (bool, error) {
	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer4 {
		t.logger.Debugf("Can delete whole listener %s, because it is layer4 listener.", listener.UUID)
		return true, nil
	}

	currentPolicies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listener.UUID)
	if err != nil {
		return false, fmt.Errorf("list policies of listener %s on LB %s: %w", listener.UUID, lbId, err)
	}

	createdPolicies := []v1alpha1.CreatedPolicy{}
	for _, l := range createdListeners {
		if l.Id == listener.UUID {
			createdPolicies = l.CreatedPolicies
			break
		}
	}

	// check if all current policies exist in created policies (created by me)
	for _, currentPolicy := range currentPolicies.Items {
		found := false
		for _, createdPolicy := range createdPolicies {
			if createdPolicy.Id == currentPolicy.UUID {
				found = true
				break
			}
		}
		if !found {
			t.logger.Debugf("Can't delete whole listener, found policy not created by me: %s", currentPolicy.Name)
			return false, nil
		}
	}

	if listener.DefaultPoolId != "" {
		// check if default pool exists in new created pools
		for _, createdPool := range newCreatedPools {
			if createdPool.Id == listener.DefaultPoolId {
				t.logger.Debugf("Cannot delete whole listener %s, because default pool %s exists in new created pools.", listener.UUID, createdPool.Id)
				return false, nil
			}
		}

		// check if default pool is created by me
		found := false
		createdPool := v1alpha1.CreatedPool{}
		for _, p := range t.lbConfig.Status.CreatedPools {
			if p.Id == listener.DefaultPoolId {
				found = true
				createdPool = p
				break
			}
		}
		if !found {
			t.logger.Debugf("Can't delete whole listener, default pool %s not created by me.", listener.DefaultPoolId)
			return false, nil
		}

		// compare pool members
		currentListMembers, err := t.vngcloudRepo.GetPoolMembers(ctx, lbId, listener.DefaultPoolId)
		if err != nil {
			return false, fmt.Errorf("get members of pool %s on LB %s: %w", listener.DefaultPoolId, lbId, err)
		}
		canDeleteWhole, _ := t.canDeleteWholePool(ctx, lbId, listener.DefaultPoolId, currentListMembers, createdPool.CreatedMembers, []v1alpha1.PoolMember{})
		if !canDeleteWhole {
			t.logger.Debugf("Can't delete whole listener, default pool %s cannot be deleted whole.", listener.DefaultPoolId)
			return false, nil
		}
	}

	t.logger.Debugf("Can delete whole listener %s.", listener.UUID)
	return true, nil
}
