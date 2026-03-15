package glbc_uc

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// delete redundant listeners
// newCreatedListeners is the listeners that are still in use
func (t *defaultModelDeployTask) deleteRedundantListeners(ctx context.Context, lbId string, newCreatedListeners []v1alpha1.CreatedGlobalListener, newCreatedPools []v1alpha1.CreatedGlobalPool) error {
	// delete candidates include all created listeners
	deleteCandidates := make([]string, 0)
	for _, listener := range t.lbConfig.Status.CreatedListeners {
		deleteCandidates = append(deleteCandidates, listener.Id)
	}

	currentListeners, err := t.vngcloudRepo.ListGlobalListeners(ctx, lbId)
	if err != nil {
		return err
	}

	isListenerExist := func(listenerId string) (bool, *entityv2.GlobalListener) {
		for _, l := range currentListeners.Items {
			if l.ID == listenerId {
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
	for _, candidateId := range deleteCandidates {
		isExist, listener := isListenerExist(candidateId)
		if !isExist {
			t.logger.Warnf("Listener %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		canDeleteWhole, err := t.canDeleteWholeListener(ctx, lbId, listener, newCreatedPools)
		if err != nil {
			return err
		}

		if !isListenerInUse(candidateId) && canDeleteWhole {
			if err := t.vngcloudRepo.DeleteGlobalListener(ctx, lbId, candidateId); err != nil {
				t.logger.Error("Failed to delete listener: ", err)
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

// canDeleteWholeListener checks if we can delete the whole listener.
// Returns true only when this GLBC owns all pool member groups and all individual
// members in the listener's pool (bottom-up ownership check).
//
// Logic:
//  1. If listener has no pool (GlobalPoolID is empty) -> can delete (true)
//  2. If pool is still in new spec (newCreatedPools) -> cannot delete (false): pool still in use
//  3. If pool is not in status.createdPools -> cannot delete (false): not our pool
//  4. Fetch current pool members from API
//  5. For each current PoolMember group: verify group name is in status
//  6. For each individual member in a group: verify address+port is in status
func (t *defaultModelDeployTask) canDeleteWholeListener(ctx context.Context, lbId string, listener *entityv2.GlobalListener, newCreatedPools []v1alpha1.CreatedGlobalPool) (bool, error) {
	// Step 1: No pool attached — safe to delete the whole listener
	if listener.GlobalPoolID == "" {
		t.logger.Debugf("Can delete whole listener %s: no pool attached", listener.ID)
		return true, nil
	}

	// Step 2: Pool is still referenced by the new spec — listener is still in use
	for _, p := range newCreatedPools {
		if p.Id == listener.GlobalPoolID {
			t.logger.Debugf("Cannot delete whole listener %s: pool %s is still in new spec", listener.ID, listener.GlobalPoolID)
			return false, nil
		}
	}

	// Step 3: Find the pool in our status; if absent, it's not ours
	var statusPool *v1alpha1.CreatedGlobalPool
	for i := range t.lbConfig.Status.CreatedPools {
		if t.lbConfig.Status.CreatedPools[i].Id == listener.GlobalPoolID {
			statusPool = &t.lbConfig.Status.CreatedPools[i]
			break
		}
	}
	if statusPool == nil {
		t.logger.Debugf("Cannot delete whole listener %s: pool %s not in status", listener.ID, listener.GlobalPoolID)
		return false, nil
	}

	// Step 4: Fetch current PoolMember groups from VNG Cloud
	currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, listener.GlobalPoolID)
	if err != nil {
		t.logger.Errorf("Failed to list pool members for pool %s: %v", listener.GlobalPoolID, err)
		return false, err
	}

	// Step 5: Build a map of created PoolMember group names for fast lookup
	// map: groupName -> slice of owned members (Address+Port)
	type memberKey struct {
		address string
		port    int
	}
	ownedGroups := make(map[string]map[memberKey]bool)
	for _, pm := range statusPool.CreatedPoolMembers {
		members := make(map[memberKey]bool)
		for _, m := range pm.CreatedMembers {
			members[memberKey{address: m.Address, port: m.Port}] = true
		}
		ownedGroups[pm.Name] = members
	}

	// Step 6: Check each current PoolMember group and its individual members
	for _, currentGroup := range currentPoolMembers.Items {
		ownedMembers, groupOwned := ownedGroups[currentGroup.Name]
		if !groupOwned {
			t.logger.Debugf("Cannot delete whole listener %s: pool member group %s is not in status", listener.ID, currentGroup.Name)
			return false, nil
		}

		// Check each individual member by Address+Port
		if currentGroup.Members != nil {
			for _, m := range currentGroup.Members.Items {
				key := memberKey{address: m.Address, port: m.Port}
				if !ownedMembers[key] {
					t.logger.Debugf("Cannot delete whole listener %s: member %s:%d in group %s is not in status", listener.ID, m.Address, m.Port, currentGroup.Name)
					return false, nil
				}
			}
		}
	}

	t.logger.Debugf("Can delete whole listener %s: all pool members are owned by this GLBC", listener.ID)
	return true, nil
}
