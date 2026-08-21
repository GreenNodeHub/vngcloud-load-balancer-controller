package lbc_uc

import (
	"context"
	"errors"
	"fmt"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// errPartialDelete marks a pass in which some pools were cleaned up and others were not. The
// caller must still treat it as a failure - the load balancer is not in the state we asked for
// - but the pools that did come off stay off, and the next pass has less to do.
var errPartialDelete = errors.New("some pools could not be cleaned up")

// delete pools created not in use anymore
// should check if pool is used by other listeners or policies (user use) then ignore
func (t *defaultModelDeployTask) deleteRedundantPools(ctx context.Context, lbId string, newCreatedPools []v1alpha1.CreatedPool) error {
	deleteCandidates := make([]string, 0)
	for _, pool := range t.lbConfig.Status.CreatedPools {
		deleteCandidates = append(deleteCandidates, pool.Id)
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

	// find pools in use by listeners and policies
	mapPoolInUse := make(map[string]bool)
	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list listeners of load balancer: ", err)
		return err
	}
	for _, listener := range currentListeners.Items {
		mapPoolInUse[listener.DefaultPoolId] = true
		if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer7 {
			// check listener policies
			policies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listener.UUID)
			if err != nil {
				t.logger.Error("Failed to list policies of listener: ", err)
				return err
			}
			for _, policy := range policies.Items {
				if policy.RedirectPoolID != "" {
					mapPoolInUse[policy.RedirectPoolID] = true
				}
			}
		}
	}

	isPoolInUse := func(poolId string) bool {
		if _, ok := mapPoolInUse[poolId]; ok {
			return true
		}
		return false
	}

	// One stubborn pool must not shield the others. Each candidate is independent - different
	// pool, different members - and the failure that showed up in production was one pool
	// returning VngCloudVLBLoadBalancerNotReady, which aborted the loop before the remaining
	// four were touched and then aborted at the same pool on every retry. Failures are
	// collected and returned at the end, so the reconcile still retries.
	failures := make([]error, 0)

	for _, candidateId := range deleteCandidates {
		if !isPoolExist(candidateId) {
			t.logger.Warnf("Pool %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		createdMembers := []v1alpha1.PoolMember{}
		for _, m := range t.lbConfig.Status.CreatedPools {
			if m.Id == candidateId && len(m.CreatedMembers) > 0 {
				createdMembers = m.CreatedMembers
				break
			}
		}

		newCreatedMembers := []v1alpha1.PoolMember{}
		for _, m := range newCreatedPools {
			if m.Id == candidateId && len(m.CreatedMembers) > 0 {
				newCreatedMembers = m.CreatedMembers
				break
			}
		}

		currentListMembers, err := t.vngcloudRepo.GetPoolMembers(ctx, lbId, candidateId)
		if err != nil {
			t.logger.Errorf("Failed to get pool members for pool %s: %v", candidateId, err)
			failures = append(failures, fmt.Errorf("pool %s: get members: %s", candidateId, err.Error()))
			continue
		}

		canDeleteWhole, updateMemberOption := t.canDeleteWholePool(ctx, lbId, candidateId, currentListMembers, createdMembers, newCreatedMembers)

		if !isPoolInUse(candidateId) && canDeleteWhole {
			// delete pool
			err := t.retryOnLoadBalancerNotReady(ctx, lbId, func() error {
				return t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)
			})
			if err != nil {
				t.logger.Error("Failed to delete pool: ", err)
				failures = append(failures, fmt.Errorf("pool %s: delete: %s", candidateId, err.Error()))
				continue
			}
			if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				failures = append(failures, fmt.Errorf("pool %s: wait after delete: %s", candidateId, err.Error()))
				continue
			}
		} else if updateMemberOption != nil {
			// update to delete redundant members
			t.logger.Debugf("Update pool %s members to remove redundant members", candidateId)
			err := t.retryOnLoadBalancerNotReady(ctx, lbId, func() error {
				return t.vngcloudRepo.UpdatePoolMembers(ctx, lbId, candidateId, updateMemberOption)
			})
			if err != nil {
				t.logger.Error("Failed to update pool members: ", err)
				failures = append(failures, fmt.Errorf("pool %s: update members: %s", candidateId, err.Error()))
				continue
			}
			if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				failures = append(failures, fmt.Errorf("pool %s: wait after update: %s", candidateId, err.Error()))
				continue
			}
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%w: %w", errPartialDelete, errors.Join(failures...))
	}
	return nil
}

// canDeleteWholePool checks if we can delete the whole pool
// conditions:
// - all members of the pool are created by us and not in new created members
func (t *defaultModelDeployTask) canDeleteWholePool(ctx context.Context, lbId, poolId string, currentListMembers *entity.ListMembers, createdMembers, newCreatedMembers []v1alpha1.PoolMember) (bool, loadbalancerv2.IUpdatePoolMembersRequest) {
	updateMembers := t.mergePoolMembers(ctx,
		createdMembers,
		convertMemberList(currentListMembers),
		newCreatedMembers)

	if len(updateMembers) == 0 {
		t.logger.Infof("Can delete whole pool %s, all members are created by us and not in new created members", poolId)
		return true, nil
	}

	if !t.comparePoolMembers(ctx, updateMembers, convertMemberList(currentListMembers)) {
		convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
		for _, member := range updateMembers {
			convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IP, member.Port, member.MonitorPort))
		}
		updateMemberOptions := loadbalancerv2.NewUpdatePoolMembersRequest(lbId, poolId).WithMembers(convertMembers...)
		return false, updateMemberOptions
	}

	return false, nil
}
