package glbc_uc

import (
	"context"

	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// delete pools created not in use anymore
// should check if pool is used by other listeners or policies (user use) then ignore
func (t *defaultModelDeployTask) deleteRedundantPools(ctx context.Context, lbId string, newCreatedPools []v1alpha1.CreatedGlobalPool) error {
	deleteCandidates := make([]string, 0)
	for _, pool := range t.lbConfig.Status.CreatedPools {
		deleteCandidates = append(deleteCandidates, pool.Id)
	}

	currentPools, err := t.vngcloudRepo.ListGlobalPools(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list pools of load balancer: ", err)
		return err
	}
	isPoolExist := func(poolId string) bool {
		for _, p := range currentPools.Items {
			if p.ID == poolId {
				return true
			}
		}
		return false
	}

	// find pools in use by listeners and policies
	mapPoolInUse := make(map[string]bool)
	currentListeners, err := t.vngcloudRepo.ListGlobalListeners(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list listeners of load balancer: ", err)
		return err
	}
	for _, listener := range currentListeners.Items {
		mapPoolInUse[listener.GlobalPoolID] = true
	}

	isPoolInUse := func(poolId string) bool {
		if _, ok := mapPoolInUse[poolId]; ok {
			return true
		}
		return false
	}

	for _, candidateId := range deleteCandidates {
		if !isPoolExist(candidateId) {
			t.logger.Warnf("Pool %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		createdPool := &v1alpha1.CreatedGlobalPool{}
		for _, m := range t.lbConfig.Status.CreatedPools {
			if m.Id == candidateId {
				createdPool = &m
				break
			}
		}

		newCreatedPool := &v1alpha1.CreatedGlobalPool{}
		for _, m := range newCreatedPools {
			if m.Id == candidateId {
				newCreatedPool = &m
				break
			}
		}

		canDeleteWhole, updateMemberOption, err := t.canDeleteWholePool(ctx, lbId, candidateId, createdPool, newCreatedPool)
		if err != nil {
			return err
		}

		if !isPoolInUse(candidateId) && canDeleteWhole {
			// delete pool
			err := t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)
			if err != nil {
				t.logger.Error("Failed to delete pool: ", err)
				return err
			}
			if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		} else if updateMemberOption != nil {
			// update to delete redundant pool members
			t.logger.Debugf("Update pool %s to remove redundant pool members", candidateId)
			if err = t.vngcloudRepo.PatchGlobalPoolMembers(ctx, lbId, candidateId, updateMemberOption); err != nil {
				t.logger.Error("Failed to patch pool members: ", err)
				return err
			}
			if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}
	return nil
}

// canDeleteWholePool checks if we can delete the whole pool
// conditions:
// - all pool members of the pool are created by us and not in new created pool members
func (t *defaultModelDeployTask) canDeleteWholePool(ctx context.Context, lbId, poolId string, createdPool, newCreatedPool *v1alpha1.CreatedGlobalPool) (bool, global.IPatchGlobalPoolMembersRequest, error) {
	// ensure pool members
	currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)
	if err != nil {
		t.logger.Error("Failed to list pool members: ", err)
		return false, nil, err
	}

	// Extract created pool member names
	createdPoolMemberNames := make(map[string]bool)
	for _, pm := range createdPool.CreatedPoolMembers {
		createdPoolMemberNames[pm.Name] = true
	}

	// Extract new created pool member names
	newCreatedPoolMemberNames := make(map[string]bool)
	for _, pm := range newCreatedPool.CreatedPoolMembers {
		newCreatedPoolMemberNames[pm.Name] = true
	}

	// Check which pool members should be kept
	poolMembersToKeep := make([]string, 0)
	for _, pm := range currentPoolMembers.Items {
		// Keep if: in new spec OR not created by us
		if newCreatedPoolMemberNames[pm.Name] || !createdPoolMemberNames[pm.Name] {
			poolMembersToKeep = append(poolMembersToKeep, pm.Name)
		}
	}

	if len(poolMembersToKeep) == 0 {
		t.logger.Infof("Can delete whole pool %s, all pool members are created by us and not in new created members", poolId)
		return true, nil, nil
	}

	// Check if we need to delete any pool members
	if len(poolMembersToKeep) < len(currentPoolMembers.Items) {
		// Need to delete some pool members - build a patch request
		bulkRequests := make([]global.IBulkActionRequest, 0)
		for _, pm := range currentPoolMembers.Items {
			// Delete pool members that were created by us and are not in new spec
			if createdPoolMemberNames[pm.Name] && !newCreatedPoolMemberNames[pm.Name] {
				t.logger.Debugf("Will delete pool member %s from pool %s", pm.Name, poolId)
				bulkRequests = append(bulkRequests, global.NewPatchGlobalPoolDeleteBulkActionRequest(pm.ID))
			}
		}
		if len(bulkRequests) > 0 {
			return false, global.NewPatchGlobalPoolMembersRequest(lbId, poolId).WithBulkAction(bulkRequests...), nil
		}
	}

	return false, nil, nil
}
