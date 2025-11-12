package lbc_uc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) deployListeners(ctx context.Context, lbId string, newCreatedPools []v1alpha1.CreatedPool, createdCerts []v1alpha1.CreatedCertificate) ([]v1alpha1.CreatedListener, error) {
	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		return nil, err
	}

	createdListeners := make([]v1alpha1.CreatedListener, 0)
	for _, listener := range t.lbConfig.Spec.Listeners {
		if createdListener, err := t.deployListener(ctx, lbId, listener, currentListeners, newCreatedPools, createdCerts); err != nil {
			return nil, err
		} else {
			createdListeners = append(createdListeners, *createdListener)
		}
	}
	return createdListeners, nil
}

func (t *defaultModelDeployTask) deployListener(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, currentListeners *entityv2.ListListeners, newCreatedPools []v1alpha1.CreatedPool, createdCerts []v1alpha1.CreatedCertificate) (*v1alpha1.CreatedListener, error) {
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
		createRequest, err := t.buildCreateListenerRequest(ctx, lbId, listenerSpec, newCreatedPools, createdCerts)
		if err != nil {
			return nil, err
		}
		_lis, err := t.vngcloudRepo.CreateListener(ctx, lbId, createRequest)
		if err != nil {
			t.logger.Error("Failed to create listener: ", err)
			return nil, err
		}
		if err := t.statusAddListener(ctx, _lis.UUID, _lis.ProtocolPort); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
		currentListener, err = t.vngcloudRepo.GetListenerById(ctx, lbId, _lis.UUID)
		if err != nil {
			return nil, err
		}
	}

	// if mismatch listener protocol, return error => user must delete listener in portal
	// TODO: should we do it automatically?
	if currentListener.Protocol != string(listenerSpec.Protocol) {
		t.logger.Error("Listener protocol mismatch: ", currentListener.Protocol, listenerSpec.Protocol)
		return nil, errors.New("listener port " + string(listenerSpec.ProtocolPort) + " protocol mismatch, please delete listener first in portal")
	}

	// update exist listener
	updateOptions, message, err := t.buildListenerUpdateRequest(ctx, lbId, listenerSpec, currentListener, newCreatedPools, createdCerts)
	if err != nil {
		return nil, err
	}
	if updateOptions != nil {
		t.logger.Info("Need update listener: ", strings.Join(message, ", "))
		err := t.vngcloudRepo.UpdateListener(ctx, lbId, currentListener.UUID, updateOptions)
		if err != nil {
			t.logger.Error("Failed to update listener: ", err)
			return nil, err
		}
		if err := t.statusAddListener(ctx, currentListener.UUID, currentListener.ProtocolPort); err != nil {
			return nil, err
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
			return nil, err
		}
	}

	// skip policy for layer4 listener
	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer4 {
		return &v1alpha1.CreatedListener{
			Id:   currentListener.UUID,
			Port: currentListener.ProtocolPort,
		}, nil
	}

	// ensure policy
	createdPolicies, err := t.deployPolicies(ctx, lbId, currentListener.UUID, listenerSpec.Policies, newCreatedPools)
	if err != nil {
		return nil, err
	}

	return &v1alpha1.CreatedListener{
		Id:              currentListener.UUID,
		Port:            currentListener.ProtocolPort,
		CreatedPolicies: createdPolicies,
	}, nil
}

func (t *defaultModelDeployTask) buildCreateListenerRequest(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, newCreatedPools []v1alpha1.CreatedPool, createdCerts []v1alpha1.CreatedCertificate) (loadbalancerv2.ICreateListenerRequest, error) {
	createRequest := loadbalancerv2.NewCreateListenerRequest(
		listenerSpec.Name,
		listenerSpec.Protocol,
		int(listenerSpec.ProtocolPort),
	).WithAllowedCidrs(t.cfg.LoadBalancerOpts.DefaultAllowedCidrs).
		WithTimeoutClient(t.cfg.LoadBalancerOpts.DefaultTimeoutClient).
		WithTimeoutMember(t.cfg.LoadBalancerOpts.DefaultTimeoutMember).
		WithTimeoutConnection(t.cfg.LoadBalancerOpts.DefaultTimeoutConnection).
		WithLoadBalancerId(lbId)

	if listenerSpec.AllowedCidrs != nil {
		createRequest.WithAllowedCidrs(*listenerSpec.AllowedCidrs)
	}
	if listenerSpec.TimeoutClient != nil {
		createRequest.WithTimeoutClient(int(*listenerSpec.TimeoutClient))
	}
	if listenerSpec.TimeoutMember != nil {
		createRequest.WithTimeoutMember(int(*listenerSpec.TimeoutMember))
	}
	if listenerSpec.TimeoutConnection != nil {
		createRequest.WithTimeoutConnection(int(*listenerSpec.TimeoutConnection))
	}
	if listenerSpec.DefaultPoolName != nil {
		findCreatedPoolByName := func(name string) *v1alpha1.CreatedPool {
			for _, p := range newCreatedPools {
				if p.Name == name {
					return &p
				}
			}
			return nil
		}
		createdPool := findCreatedPoolByName(*listenerSpec.DefaultPoolName)
		if createdPool != nil {
			createRequest.WithDefaultPoolId(createdPool.Id)
		} else {
			t.logger.Warnf("Default pool name %s not found in created pools", *listenerSpec.DefaultPoolName)
		}
	}

	if len(listenerSpec.InsertHeaders) > 0 {
		insertHeaders := make([]string, len(listenerSpec.InsertHeaders)*2)
		for i, h := range listenerSpec.InsertHeaders {
			insertHeaders[i*2] = h.HeaderName
			insertHeaders[i*2+1] = h.HeaderValue
		}
		createRequest.WithInsertHeaders(insertHeaders...)
	}

	if listenerSpec.CertificateDefault != nil {
		certId, err := t.findListenerCertificateId(ctx, *listenerSpec.CertificateDefault, createdCerts)
		if err != nil {
			t.logger.Errorf("failed to find default certificate %+v: %v", listenerSpec.CertificateDefault, err)
			return nil, err
		}
		createRequest.WithDefaultCertificateAuthority(&certId)
	}

	if len(listenerSpec.CertificateAuthorities) > 0 {
		certificateAuthorities := make([]string, 0)
		for _, ca := range listenerSpec.CertificateAuthorities {
			certId, err := t.findListenerCertificateId(ctx, ca, createdCerts)
			if err != nil {
				t.logger.Errorf("failed to find certificate authority %+v: %v", ca, err)
				return nil, err
			}
			certificateAuthorities = append(certificateAuthorities, certId)
		}
		createRequest.WithCertificateAuthorities(&certificateAuthorities)
	}

	if listenerSpec.ClientCertificateId != nil {
		createRequest.WithClientCertificate(listenerSpec.ClientCertificateId)
	}

	// Policy not supported create along with listener creation yet

	return createRequest, nil
}

func (t *defaultModelDeployTask) buildListenerUpdateRequest(ctx context.Context, lbId string, listenerSpec v1alpha1.Listener, currentListener *entityv2.Listener, newCreatedPools []v1alpha1.CreatedPool, createdCerts []v1alpha1.CreatedCertificate) (loadbalancerv2.IUpdateListenerRequest, []string, error) {
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
		DefaultCertificateAuthority: currentListener.DefaultCertificateAuthority,
		CertificateAuthorities:      &currentListener.CertificateAuthorities,
		InsertHeaders:               &currentListener.InsertHeaders,
		ClientCertificate:           currentListener.ClientCertificateAuthentication,
	}

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
		findCreatedPoolByName := func(name string) *v1alpha1.CreatedPool {
			for _, p := range newCreatedPools {
				if p.Name == name {
					return &p
				}
			}
			return nil
		}
		createdPool := findCreatedPoolByName(*listenerSpec.DefaultPoolName)
		if createdPool != nil {
			message = append(message, fmt.Sprintf("default pool (%s -> %s)", currentListener.DefaultPoolName, *listenerSpec.DefaultPoolName))
			updateOptions.WithDefaultPoolId(createdPool.Id)
			isNeedUpdate = true
		} else {
			t.logger.Warnf("Default pool name %s not found in created pools", *listenerSpec.DefaultPoolName)
		}
	}

	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer7 {

		// headers
		if !t.compareHeader(currentListener.InsertHeaders, listenerSpec.InsertHeaders) {
			message = append(message, fmt.Sprintf("headers (%v -> %v)", currentListener.InsertHeaders, listenerSpec.InsertHeaders))
			updateInsertHeaders := []entityv2.ListenerInsertHeader{}
			for _, h := range listenerSpec.InsertHeaders {
				updateInsertHeaders = append(updateInsertHeaders, entityv2.ListenerInsertHeader{
					HeaderName:  h.HeaderName,
					HeaderValue: h.HeaderValue,
				})
			}
			updateOptions.InsertHeaders = &updateInsertHeaders
			isNeedUpdate = true
		}

		if listenerSpec.Protocol == loadbalancerv2.ListenerProtocolHTTPS {

			// client certificate
			if !comparePointer(currentListener.ClientCertificateAuthentication, listenerSpec.ClientCertificateId) {
				message = append(message, fmt.Sprintf("client certificate (%v -> %v)",
					pointerToString(currentListener.ClientCertificateAuthentication), pointerToString(listenerSpec.ClientCertificateId)))
				updateOptions.ClientCertificate = listenerSpec.ClientCertificateId
				isNeedUpdate = true
			}

			// default certificate authority
			if listenerSpec.CertificateDefault == nil {
				if currentListener.DefaultCertificateAuthority != nil {
					message = append(message, fmt.Sprintf("default certificate authority (%s -> %v)",
						pointerToString(currentListener.DefaultCertificateAuthority), nil))
					updateOptions.DefaultCertificateAuthority = nil
					isNeedUpdate = true
				}
			} else {
				certId, err := t.findListenerCertificateId(ctx, *listenerSpec.CertificateDefault, createdCerts)
				if err != nil {
					return nil, nil, err
				}
				if currentListener.DefaultCertificateAuthority == nil || *currentListener.DefaultCertificateAuthority != certId {
					message = append(message, fmt.Sprintf("default certificate authority (%s -> %s)",
						pointerToString(currentListener.DefaultCertificateAuthority), certId))
					updateOptions.DefaultCertificateAuthority = &certId
					isNeedUpdate = true
				}
			}

			// certificate authorities
			if len(listenerSpec.CertificateAuthorities) == 0 {
				if len(currentListener.CertificateAuthorities) > 0 {
					message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", currentListener.CertificateAuthorities, nil))
					updateOptions.CertificateAuthorities = nil
					isNeedUpdate = true
				}
			} else {
				newCAs := []string{}
				for _, ca := range listenerSpec.CertificateAuthorities {
					certId, err := t.findListenerCertificateId(ctx, ca, createdCerts)
					if err != nil {
						return nil, nil, err
					}
					newCAs = append(newCAs, certId)
				}
				if !sets.New(newCAs...).Equal(sets.New(currentListener.CertificateAuthorities...)) {
					message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", currentListener.CertificateAuthorities, newCAs))
					updateOptions.CertificateAuthorities = &newCAs
					isNeedUpdate = true
				}
			}
		}
	}

	if !isNeedUpdate {
		return nil, nil, nil
	}
	return updateOptions, message, nil
}

// delete redundant listeners
// newCreatedListeners is the listeners that are still in use
func (t *defaultModelDeployTask) deployDeleteRedundantListeners(ctx context.Context, lbId string, newCreatedListeners []v1alpha1.CreatedListener, status v1alpha1.LoadBalancerConfigStatus) error {
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
		for _, listener := range newCreatedListeners {
			if listener.Id == listenerId {
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

// compare listener headers, if they are the same then return true
func (t *defaultModelDeployTask) compareHeader(current []entityv2.ListenerInsertHeader, newHeaders []v1alpha1.InsertHeader) bool {
	if current == nil && newHeaders == nil {
		return true
	}
	if current == nil || newHeaders == nil {
		return false
	}
	if len(current) != len(newHeaders) {
		return false
	}
	mapHeader := make(map[string]string)
	for _, header := range newHeaders {
		mapHeader[header.HeaderName] = header.HeaderValue
	}
	for _, header := range current {
		if _, ok := mapHeader[header.HeaderName]; !ok {
			return false
		}
		if header.HeaderValue != mapHeader[header.HeaderName] {
			return false
		}
	}
	return true
}

func (t *defaultModelDeployTask) findListenerCertificateId(ctx context.Context, cert v1alpha1.ListenerCertificate, createdCerts []v1alpha1.CreatedCertificate) (string, error) {
	if cert.Id != nil {
		return *cert.Id, nil
	}
	if cert.SecretName != nil {
		for _, createdCert := range createdCerts {
			if createdCert.SecretName == *cert.SecretName {
				return createdCert.Id, nil
			}
		}
		return "", errors.Errorf("certificate with secret name %s not found in created certificates", *cert.SecretName)
	}
	if cert.Name != nil {
		certList, err := t.vngcloudRepo.ListCertificates(ctx)
		if err != nil {
			return "", err
		}
		for _, c := range certList.Certificates {
			if c.Name == *cert.Name {
				return c.UUID, nil
			}
		}
		return "", errors.Errorf("certificate with name %s not found in vngcloud", *cert.Name)
	}
	return "", errors.New("listener certificate must have id, secretName or name")
}

func comparePointer[T comparable](current, new *T) bool {
	if current == nil && new == nil {
		return true
	}
	if current == nil || new == nil {
		return false
	}
	return *current == *new
}

func pointerToString[T any](p *T) string {
	if p == nil {
		return "(nil)"
	}
	return fmt.Sprintf("&(%v)", *p)
}
