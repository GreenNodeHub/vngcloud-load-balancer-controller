package lbc_uc

import "context"

func (t *defaultModelDeleteTask) deleteRedundantListeners(ctx context.Context, lbId string) error {
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

	// delete redundant listeners
	for _, candidateId := range deleteCandidates {
		if !isListenerExist(candidateId) {
			t.logger.Warnf("Listener %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		t.logger.Infof("Deleting redundant listener %s", candidateId)
		err := t.vngcloudRepo.DeleteListener(ctx, lbId, candidateId)
		if err != nil {
			t.logger.Error("Failed to delete listener: ", err)
			return err
		}
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}

		// TODO
		// // delete whole listener if new not used and can delete whole
		// if newListener == nil && r.CanDeleteWholeListener(oldListener) {
		// 	if err := r.provider.DeleteListener(r.context, r.GetLoadBalancerID(), oldListener.GetID()); err != nil {
		// 		r.logger.Error("Failed to delete listener: ", err)
		// 		return err
		// 	}
		// 	if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
		// 		r.logger.Error("Failed to wait for loadbalancer active: ", err)
		// 		return err
		// 	}
		// }

		// TODO
		// // delete redundant policy
		// if err := r.deleteRedundantPolicies(oldListener, currentListener, newListener); err != nil {
		// 	return err
		// }
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

// func (t *defaultModelDeleteTask)

// func (t *defaultModelDeleteTask)

// func (t *defaultModelDeleteTask)
