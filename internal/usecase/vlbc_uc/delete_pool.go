package vlbc_uc

import "context"

// delete redundant pools, should check if pool is used by other listeners or policy then ignore
func (t *defaultModelDeleteTask) deleteRedundantPools(ctx context.Context, lbId string) error {
	deleteCandidates := make([]string, 0)
	for _, pool := range t.vlbConfig.Status.CreatedPools {
		deleteCandidates = append(deleteCandidates, pool.Id)
	}

	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list listeners of load balancer: ", err)
		return err
	}
	currentPools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list pools of load balancer: ", err)
		return err
	}
	isPoolExist := func(poolId string) bool {
		for _, p := range currentPools.Items {
			if p.UUID == poolId {
				return true
			}
		}
		return false
	}

	isPoolInUse := func(poolId string) bool {
		for _, listener := range currentListeners.Items {
			if listener.DefaultPoolId == poolId {
				return true
			}
		}
		return false
	}

	for _, candidateId := range deleteCandidates {
		if isPoolInUse(candidateId) {
			continue
		}
		if !isPoolExist(candidateId) {
			t.logger.Warnf("Pool %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		// delete pool
		err := t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)
		if err != nil {
			t.logger.Error("Failed to delete pool: ", err)
			return err
		}
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}
	return nil
}

// func (t *defaultModelDeleteTask)

// func (t *defaultModelDeleteTask)

// func (t *defaultModelDeleteTask)

// func (t *defaultModelDeleteTask)
