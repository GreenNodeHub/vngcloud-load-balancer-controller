package lbc_uc

import (
	"context"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// deployDeleteRedundantPoliciesFrom removes the listener's policies this LBC created and no
// longer wants. The created-listener record is a parameter rather than read from status; see
// deleteRedundantPoolsFrom for why.
func (t *defaultModelDeployTask) deployDeleteRedundantPoliciesFrom(ctx context.Context, lbId string, createdListeners []v1alpha1.CreatedListener, listenerId string, newCreatedPolicies []v1alpha1.CreatedPolicy) error {
	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer4 {
		return nil
	}

	createdPolicies := []v1alpha1.CreatedPolicy{}
	for _, l := range createdListeners {
		if l.Id == listenerId {
			createdPolicies = l.CreatedPolicies
			break
		}
	}

	if len(createdPolicies) == 0 {
		return nil
	}

	// delete candidates include all created listeners
	deleteCandidates := make([]string, 0)
	for _, policy := range createdPolicies {
		deleteCandidates = append(deleteCandidates, policy.Id)
	}

	currentPolicies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listenerId)
	if err != nil {
		return err
	}

	isPolicyExist := func(id string) bool {
		for _, l := range currentPolicies.Items {
			if l.UUID == id {
				return true
			}
		}
		return false
	}

	isPolicyInUse := func(id string) bool {
		for _, policy := range newCreatedPolicies {
			if policy.Id == id {
				return true
			}
		}
		return false
	}

	// delete redundant policies
	for _, candidateId := range deleteCandidates {
		if isPolicyInUse(candidateId) {
			continue
		}
		if !isPolicyExist(candidateId) {
			continue
		}

		if err := t.vngcloudRepo.DeletePolicy(ctx, lbId, listenerId, candidateId); err != nil {
			t.logger.Error("Failed to delete policy: ", err)
			return err
		}
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}
	return nil
}
