package lbc_uc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) deployListeners(ctx context.Context, lbId string, mapPoolNameToID map[string]string) (map[int]string, error) {
	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return nil, err
	}

	mapListenerPortToID := make(map[int]string)
	for _, listener := range t.lbConfig.Spec.Listeners {
		if listenerId, err := t.deployListener(ctx, lbId, listener, currentListeners, mapPoolNameToID); err != nil {
			return nil, err
		} else {
			mapListenerPortToID[int(listener.ProtocolPort)] = listenerId
		}
	}
	return mapListenerPortToID, nil
}

func (t *defaultModelDeployTask) deployListener(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, currentListeners *entityv2.ListListeners, mapPoolNameToID map[string]string) (string, error) {
	searchListenerByPort := func(port int) *entityv2.Listener {
		for _, l := range currentListeners.Items {
			if l.ProtocolPort == port {
				return l
			}
		}
		return nil
	}

	currentListener := searchListenerByPort(int(listenerSpec.ProtocolPort))
	if currentListener == nil {
		_lis, err := t.vngcloudRepo.CreateListener(ctx, lbId,
			t.buildCreateListenerRequest(ctx, lbId, listenerSpec, mapPoolNameToID),
		)
		if err != nil {
			t.logger.Error("Failed to create listener: ", err)
			return "", err
		}
		// TODO: why I need this?
		//  else {
		// 	listenerBuilder.SetID(_lis.UUID)
		// }
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return "", err
		}
		return _lis.UUID, nil
		// TODO: Why I need it?
		// // need to update to current builder, avoid mismatch data later
		// t.AddCloneListenerBuilder(listenerBuilder)
		// listenerInPortal = t.GetListenerBuilderByPort(listenerBuilder.ListenerProtocolPort)
	}

	// if mismatch listener protocol, return error => user must delete listener in portal
	// TODO: should we do it automatically?
	if currentListener.Protocol != string(listenerSpec.Protocol) {
		t.logger.Error("Listener protocol mismatch: ", currentListener.Protocol, listenerSpec.Protocol)
		return "", errors.New("listener port " + string(listenerSpec.ProtocolPort) + " protocol mismatch, please delete listener first in portal")
	}

	// update exist listener
	updateOptions, message := t.buildListenerUpdateRequest(ctx, lbId, listenerSpec, currentListener, mapPoolNameToID)
	if updateOptions != nil {
		t.logger.Info("Need update listener: ", strings.Join(message, ", "))
		err := t.vngcloudRepo.UpdateListener(ctx, lbId, currentListener.UUID, updateOptions)
		if err != nil {
			t.logger.Error("Failed to update listener: ", err)
			return "", err
		}

		// TODO: why I need this?
		// // need to update to current builder, avoid mismatch data later
		// listenerInPortal.DefaultPoolId = &updateOptions.DefaultPoolId
		// listenerInPortal.ReferPoolName = ""
		// if p := t.GetPoolBuilderByID(updateOptions.DefaultPoolId); p != nil { // in ensurePool, should update to the latest information
		// 	listenerInPortal.ReferPoolName = p.GetName()
		// }
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return "", err
		}
	}

	// TODO
	// // ensure policy
	// for _, policyBuilder := range listenerBuilder.GetPolicyBuilders() {
	// 	// set policy redirect to pool id if exists
	// 	if policyBuilder.ReferPoolName != "" {
	// 		referPool := r.GetPoolBuilderByName(policyBuilder.ReferPoolName)
	// 		if referPool == nil {
	// 			r.logger.Error("Failed to get refer pool: ", policyBuilder.ReferPoolName)
	// 			return nil
	// 		}
	// 		policyBuilder.RedirectPoolID = referPool.GetID()
	// 	}

	// 	policyInPortal := listenerInPortal.GetPolicyBuilderByName(policyBuilder.GetName())
	// 	if policyInPortal == nil {
	// 		if _, err := r.provider.CreatePolicy(r.context, lbId, listenerInPortal.GetID(),
	// 			policyBuilder.GetICreatePolicyRequest(lbId, listenerInPortal.GetID())); err != nil {
	// 			r.logger.Error("Failed to create policy: ", err)
	// 			return err
	// 		}
	// 		if _, err := r.provider.WaitForLBActive(r.context, lbId); err != nil {
	// 			r.logger.Error("Failed to wait for loadbalancer active: ", err)
	// 			return err
	// 		}
	// 	} else {
	// 		policyBuilder.SetID(policyInPortal.GetID())
	// 		updateOptions, message := policyInPortal.ComparePolicyBuilder(lbId, listenerBuilder.GetID(), policyBuilder)
	// 		if updateOptions != nil {
	// 			r.logger.Info("Need update policy: ", strings.Join(message, ", "))
	// 			err := r.provider.UpdatePolicy(r.context, lbId, listenerBuilder.GetID(), policyInPortal.GetID(), updateOptions)
	// 			if err != nil {
	// 				r.logger.Error("Failed to update policy: ", err)
	// 				return err
	// 			}

	// 			// need to update to current builder, avoid mismatch data later
	// 			policyInPortal.ReferPoolName = ""
	// 			policyInPortal.RedirectPoolID = updateOptions.RedirectPoolID
	// 			if policyInPortal.RedirectPoolID != "" {
	// 				if p := r.GetPoolBuilderByID(updateOptions.RedirectPoolID); p != nil {
	// 					policyInPortal.ReferPoolName = p.GetName()
	// 				}
	// 			}
	// 			if _, err := r.provider.WaitForLBActive(r.context, lbId); err != nil {
	// 				r.logger.Error("Failed to wait for loadbalancer active: ", err)
	// 				return err
	// 			}
	// 		}
	// 	}
	// }

	return currentListener.UUID, nil
}

func (t *defaultModelDeployTask) buildCreateListenerRequest(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, mapPoolNameToID map[string]string) loadbalancerv2.ICreateListenerRequest {
	result := loadbalancerv2.NewCreateListenerRequest(
		listenerSpec.Name,
		listenerSpec.Protocol,
		int(listenerSpec.ProtocolPort),
	).WithAllowedCidrs(t.cfg.LoadBalancerOpts.DefaultAllowedCidrs).
		WithTimeoutClient(t.cfg.LoadBalancerOpts.DefaultTimeoutClient).
		WithTimeoutMember(t.cfg.LoadBalancerOpts.DefaultTimeoutMember).
		WithTimeoutConnection(t.cfg.LoadBalancerOpts.DefaultTimeoutConnection).
		WithLoadBalancerId(lbId)

	if listenerSpec.AllowedCidrs != nil {
		result = result.WithAllowedCidrs(*listenerSpec.AllowedCidrs)
	}
	if listenerSpec.TimeoutClient != nil {
		result = result.WithTimeoutClient(int(*listenerSpec.TimeoutClient))
	}
	if listenerSpec.TimeoutMember != nil {
		result = result.WithTimeoutMember(int(*listenerSpec.TimeoutMember))
	}
	if listenerSpec.TimeoutConnection != nil {
		result = result.WithTimeoutConnection(int(*listenerSpec.TimeoutConnection))
	}
	if listenerSpec.DefaultPoolName != nil {
		if poolId, ok := mapPoolNameToID[*listenerSpec.DefaultPoolName]; ok {
			result = result.WithDefaultPoolId(poolId)
		} else {
			t.logger.Warnf("Default pool name %s not found in mapPoolNameToID", *listenerSpec.DefaultPoolName)
		}
	}
	return result
}

func (t *defaultModelDeployTask) buildListenerUpdateRequest(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, currentListener *entityv2.Listener, mapPoolNameToID map[string]string) (loadbalancerv2.IUpdateListenerRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdateListenerRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbId,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: currentListener.UUID,
		},
		AllowedCidrs:                currentListener.AllowedCidrs,
		TimeoutClient:               currentListener.TimeoutClient,
		TimeoutMember:               currentListener.TimeoutMember,
		TimeoutConnection:           currentListener.TimeoutConnection,
		DefaultPoolId:               currentListener.DefaultPoolId,
		DefaultCertificateAuthority: nil,
		CertificateAuthorities:      nil,
		InsertHeaders:               nil,
		ClientCertificate:           nil,
	}

	// TODO
	// // set current value
	// if !new.IsL4 {
	// 	updateOptions.InsertHeaders = new.InsertHeaders
	// 	if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {
	// 		updateOptions.ClientCertificate = new.ClientCertificate
	// 		updateOptions.DefaultCertificateAuthority = new.DefaultCertificateAuthority
	// 		updateOptions.CertificateAuthorities = new.CertificateAuthorities
	// 	}
	// }

	if listenerSpec.AllowedCidrs != nil && *listenerSpec.AllowedCidrs != "" && currentListener.AllowedCidrs != *listenerSpec.AllowedCidrs {
		message = append(message, fmt.Sprintf("allowed cidrs (%v -> %v)", currentListener.AllowedCidrs, *listenerSpec.AllowedCidrs))
		updateOptions.AllowedCidrs = *listenerSpec.AllowedCidrs
		isNeedUpdate = true
	}

	if listenerSpec.TimeoutClient != nil && currentListener.TimeoutClient != int(*listenerSpec.TimeoutClient) {
		message = append(message, fmt.Sprintf("timeout client (%d -> %d)", currentListener.TimeoutClient, *listenerSpec.TimeoutClient))
		updateOptions.TimeoutClient = int(*listenerSpec.TimeoutClient)
		isNeedUpdate = true
	}

	if listenerSpec.TimeoutMember != nil && currentListener.TimeoutMember != int(*listenerSpec.TimeoutMember) {
		message = append(message, fmt.Sprintf("timeout member (%d -> %d)", currentListener.TimeoutMember, *listenerSpec.TimeoutMember))
		updateOptions.TimeoutMember = int(*listenerSpec.TimeoutMember)
		isNeedUpdate = true
	}

	if listenerSpec.TimeoutConnection != nil && currentListener.TimeoutConnection != int(*listenerSpec.TimeoutConnection) {
		message = append(message, fmt.Sprintf("timeout connection (%d -> %d)", currentListener.TimeoutConnection, *listenerSpec.TimeoutConnection))
		updateOptions.TimeoutConnection = int(*listenerSpec.TimeoutConnection)
		isNeedUpdate = true
	}

	// TODO: should update to no default pool if user set empty string
	if listenerSpec.DefaultPoolName != nil && *listenerSpec.DefaultPoolName != currentListener.DefaultPoolName {
		if poolId, ok := mapPoolNameToID[*listenerSpec.DefaultPoolName]; ok {
			message = append(message, fmt.Sprintf("default pool (%s -> %s)", currentListener.DefaultPoolName, *listenerSpec.DefaultPoolName))
			updateOptions.WithDefaultPoolId(poolId)
			isNeedUpdate = true
		} else {
			t.logger.Warnf("Default pool name %s not found in mapPoolNameToID", *listenerSpec.DefaultPoolName)
		}
	}

	// TODO
	// if !new.IsL4 {

	// 	// headers
	// 	if !current.CompareHeader(new) {
	// 		message = append(message, fmt.Sprintf("headers (%v -> %v)", *current.InsertHeaders, *new.InsertHeaders))
	// 		isNeedUpdate = true
	// 	}

	// 	if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {

	// 		// client certificate
	// 		if !comparePointer(current.ClientCertificate, new.ClientCertificate) {
	// 			message = append(message, fmt.Sprintf("client certificate (%v -> %v)",
	// 				pointerToString(current.ClientCertificate), pointerToString(new.ClientCertificate)))
	// 			isNeedUpdate = true
	// 		}

	// 		// default certificate authority
	// 		if !comparePointer(current.DefaultCertificateAuthority, new.DefaultCertificateAuthority) {
	// 			message = append(message, fmt.Sprintf("default certificate authority (%s -> %s)",
	// 				pointerToString(current.DefaultCertificateAuthority), pointerToString(new.DefaultCertificateAuthority)))
	// 			isNeedUpdate = true
	// 		}

	// 		// certificate authorities
	// 		if (current.CertificateAuthorities == nil || new.CertificateAuthorities == nil) &&
	// 			current.CertificateAuthorities != new.CertificateAuthorities {
	// 			message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
	// 			isNeedUpdate = true
	// 		} else {
	// 			// CertificateAuthorities is not nil
	// 			if len(*current.CertificateAuthorities) != len(*new.CertificateAuthorities) {
	// 				message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
	// 				isNeedUpdate = true
	// 			} else {
	// 				for _, ca := range *new.CertificateAuthorities {
	// 					if !slices.Contains(*current.CertificateAuthorities, ca) {
	// 						message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
	// 						isNeedUpdate = true
	// 						break
	// 					}
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// delete redundant listeners
// mapListenerPortToID is the listeners that are still in use
func (t *defaultModelDeployTask) deployDeleteRedundantListeners(ctx context.Context, lbId string, mapListenerPortToID map[int]string, status v1alpha1.LoadBalancerConfigStatus) error {
	// delete candidates include all created listeners
	deleteCandidates := make([]string, 0)
	for _, listener := range status.CreatedListeners {
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
		for _, id := range mapListenerPortToID {
			if id == listenerId {
				return true
			}
		}
		return false
	}

	// delete redundant listeners
	for _, candidateId := range deleteCandidates {
		if isListenerInUse(candidateId) {
			continue
		}
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
