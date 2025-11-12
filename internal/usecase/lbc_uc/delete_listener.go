package lbc_uc

import (
	"context"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// delete redundant listeners
// newCreatedListeners is the listeners that are still in use
func (t *defaultModelDeployTask) deleteRedundantListeners(ctx context.Context, lbId string, newCreatedListeners []v1alpha1.CreatedListener) error {
	// delete candidates include all created listeners
	deleteCandidates := make([]string, 0)
	for _, listener := range t.lbConfig.Status.CreatedListeners {
		deleteCandidates = append(deleteCandidates, listener.Id)
	}

	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return err
	}

	isListenerExist := func(listenerId string) bool {
		for _, l := range currentListeners.Items {
			if l.UUID == listenerId {
				return true
			}
		}
		return false
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
	for _, candidateId := range deleteCandidates {
		if !isListenerExist(candidateId) {
			t.logger.Warnf("Listener %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		canDeleteWhole, err := t.canDeleteWholeListener(ctx, lbId, candidateId)
		if err != nil {
			return err
		}

		if !isListenerInUse(candidateId) && canDeleteWhole {
			if err := t.vngcloudRepo.DeleteListener(ctx, lbId, candidateId); err != nil {
				t.logger.Error("Failed to delete listener: ", err)
				return err
			}
			if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
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
			if err := t.deployDeleteRedundantPolicies(ctx, lbId, candidateId, newCreatedPolicies); err != nil {
				return err
			}
		}
	}

	// TODO
	// // reorder policies if needed
	// if newBuilder.AutoReorderPolicies() {
	// 	for _, listener := range r.GetListenerBuilders() {
	// 		if listener.IsDeleted() {
	// 			continue
	// 		}
	// 		// get policies to update
	// 		policies, err := r.provider.ListPolicyOfListener(r.context, r.loadBalancerID, listener.GetID())
	// 		if err != nil {
	// 			return err
	// 		}
	// 		listener.policyBuilders = make([]*policyBuilderType, 0)
	// 		for _, policy := range policies.Items {
	// 			policyBuilder, err := r.buildPolicy(policy)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			listener.policyBuilders = append(listener.policyBuilders, policyBuilder)
	// 		}

	// 		// check if need reorder policies
	// 		isNeeded, policyIDs := listener.NeedReorder()
	// 		if !isNeeded {
	// 			continue
	// 		}
	// 		if err := r.provider.ReorderPolicies(r.context, r.GetLoadBalancerID(), listener.GetID(), policyIDs); err != nil {
	// 			r.logger.Error("Failed to reorder policies: ", err)
	// 			return err
	// 		}
	// 		if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
	// 			r.logger.Error("Failed to wait for loadbalancer active: ", err)
	// 			return err
	// 		}
	// 	}
	// }
	return nil
}

// canDeleteWholeListener checks if we can delete the whole listener
// conditions:
// - all current policies must exist in old policies
func (t *defaultModelDeployTask) canDeleteWholeListener(ctx context.Context, lbId, listenerId string) (bool, error) {
	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer4 {
		t.logger.Debugf("Can delete whole listener %s, because it is layer4 listener.", listenerId)
		return true, nil
	}

	currentPolicies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listenerId)
	if err != nil {
		t.logger.Errorf("Failed to list policies of listener %s: %v", listenerId, err)
		return false, err
	}

	createdPolicies := []v1alpha1.CreatedPolicy{}
	for _, l := range t.lbConfig.Status.CreatedListeners {
		if l.Id == listenerId {
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

	t.logger.Debugf("Can delete whole listener %s.", listenerId)
	return true, nil
}
