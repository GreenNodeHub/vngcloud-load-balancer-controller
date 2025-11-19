package glbc_uc

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
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

// canDeleteWholeListener checks if we can delete the whole listener
// conditions:
// - if default pool exists, can delete whole pool (case 2 ingress use same listener and default pool (merge their member), when delete one ingress, we should NOT delete whole listener)
func (t *defaultModelDeployTask) canDeleteWholeListener(ctx context.Context, lbId string, listener *entityv2.GlobalListener, newCreatedPools []v1alpha1.CreatedGlobalPool) (bool, error) {
	// TODO
	return false, domain.ErrorNotImplemented
}
