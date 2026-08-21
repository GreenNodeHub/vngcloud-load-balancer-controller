package glbc_uc

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) deployListeners(ctx context.Context, lbId string, newCreatedPools []v1alpha1.CreatedGlobalPool) ([]v1alpha1.CreatedGlobalListener, error) {
	currentListeners, err := t.vngcloudRepo.ListGlobalListeners(ctx, lbId)
	if err != nil {
		return nil, err
	}

	createdListeners := make([]v1alpha1.CreatedGlobalListener, 0)
	for _, listener := range t.lbConfig.Spec.GlobalListeners {
		if createdListener, err := t.deployListener(ctx, lbId, listener, currentListeners, newCreatedPools); err != nil {
			return nil, err
		} else {
			createdListeners = append(createdListeners, *createdListener)
		}
	}
	return createdListeners, nil
}

func (t *defaultModelDeployTask) deployListener(ctx context.Context, lbId string, listenerSpec v1alpha1.GlobalListener, currentListeners *entityv2.ListGlobalListeners, newCreatedPools []v1alpha1.CreatedGlobalPool) (*v1alpha1.CreatedGlobalListener, error) {
	searchListenerByPort := func(port int) *entityv2.GlobalListener {
		for _, l := range currentListeners.Items {
			if l.Port == port {
				return l
			}
		}
		return nil
	}

	currentListener := searchListenerByPort(listenerSpec.ProtocolPort)
	if currentListener == nil {
		createRequest, err := t.buildCreateListenerRequest(ctx, lbId, listenerSpec, newCreatedPools)
		if err != nil {
			return nil, err
		}
		_lis, err := t.vngcloudRepo.CreateGlobalListener(ctx, lbId, createRequest)
		if err != nil {
			return nil, errors.Wrapf(err, "create listener port %d on LB %s", listenerSpec.ProtocolPort, lbId)
		}
		if err := t.statusAddListener(ctx, _lis.ID, listenerSpec.ProtocolPort, listenerSpec.Name); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			return nil, errors.Wrapf(err, "wait LB %s active after creating listener %s", lbId, _lis.ID)
		}
		currentListener, err = t.vngcloudRepo.GetGlobalListener(ctx, lbId, _lis.ID)
		if err != nil {
			return nil, err
		}
	}

	if currentListener.Protocol != string(listenerSpec.Protocol) {
		return nil, errors.Errorf("listener %s port %d protocol mismatch (current %s, want %s), please delete listener first in portal",
			currentListener.ID, listenerSpec.ProtocolPort, currentListener.Protocol, listenerSpec.Protocol)
	}

	// update exist listener
	updateOptions, message, err := t.buildListenerUpdateRequest(ctx, lbId, listenerSpec, currentListener, newCreatedPools)
	if err != nil {
		return nil, err
	}
	if updateOptions != nil {
		t.logger.Infof("Need update listener %s (port %d) on LB %s: %s", currentListener.ID, currentListener.Port, lbId, strings.Join(message, ", "))
		err := t.vngcloudRepo.UpdateGlobalListener(ctx, lbId, currentListener.ID, updateOptions)
		if err != nil {
			return nil, errors.Wrapf(err, "update listener %s on LB %s", currentListener.ID, lbId)
		}
		if err := t.statusAddListener(ctx, currentListener.ID, currentListener.Port, currentListener.Name); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			return nil, errors.Wrapf(err, "wait LB %s active after updating listener %s", lbId, currentListener.ID)
		}
	}

	return &v1alpha1.CreatedGlobalListener{
		Id:   currentListener.ID,
		Port: currentListener.Port,
		Name: currentListener.Name,
	}, nil
}

func (t *defaultModelDeployTask) buildCreateListenerRequest(_ context.Context, lbId string, listenerSpec v1alpha1.GlobalListener, newCreatedPools []v1alpha1.CreatedGlobalPool) (global.ICreateGlobalListenerRequest, error) { //nolint:unparam
	createRequest := global.NewCreateGlobalListenerRequest(
		lbId,
		listenerSpec.Name,
	).WithPort(listenerSpec.ProtocolPort).
		WithAllowedCidrs(t.cfg.GlobalLoadBalancerOpts.DefaultAllowedCidrs).
		WithTimeoutClient(t.cfg.GlobalLoadBalancerOpts.DefaultTimeoutClient).
		WithTimeoutMember(t.cfg.GlobalLoadBalancerOpts.DefaultTimeoutMember).
		WithTimeoutConnection(t.cfg.GlobalLoadBalancerOpts.DefaultTimeoutConnection).
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
		findCreatedPoolByName := func(name string) *v1alpha1.CreatedGlobalPool {
			for _, p := range newCreatedPools {
				if p.Name == name {
					return &p
				}
			}
			return nil
		}
		createdPool := findCreatedPoolByName(*listenerSpec.DefaultPoolName)
		if createdPool != nil {
			createRequest.WithGlobalPoolId(createdPool.Id)
		} else {
			t.logger.Warnf("Default pool name %s not found in created pools", *listenerSpec.DefaultPoolName)
		}
	}

	if len(listenerSpec.Headers) > 0 {
		createRequest.WithHeaders(listenerSpec.Headers...)
	}

	return createRequest, nil
}

func (t *defaultModelDeployTask) buildListenerUpdateRequest(_ context.Context, lbId string, listenerSpec v1alpha1.GlobalListener, currentListener *entityv2.GlobalListener, newCreatedPools []v1alpha1.CreatedGlobalPool) (global.IUpdateGlobalListenerRequest, []string, error) { //nolint:unparam
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &global.UpdateGlobalListenerRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbId,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: currentListener.ID,
		},
		AllowedCidrs:      currentListener.AllowedCidrs,
		TimeoutClient:     currentListener.TimeoutClient,
		TimeoutMember:     currentListener.TimeoutMember,
		TimeoutConnection: currentListener.TimeoutConnection,
		GlobalPoolId:      currentListener.GlobalPoolID,
		Headers:           currentListener.Headers,
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

	// default pool can empty
	defaultPoolId := ""
	if listenerSpec.DefaultPoolName != nil && *listenerSpec.DefaultPoolName != "" {
		findCreatedPoolByName := func(name string) *v1alpha1.CreatedGlobalPool {
			for _, p := range newCreatedPools {
				if p.Name == name {
					return &p
				}
			}
			return nil
		}
		createdPool := findCreatedPoolByName(*listenerSpec.DefaultPoolName)
		if createdPool != nil {
			defaultPoolId = createdPool.Id
		}
	}
	if defaultPoolId != currentListener.GlobalPoolID {
		message = append(message, fmt.Sprintf("default pool (%s -> %s)", currentListener.GlobalPoolID, defaultPoolId))
		updateOptions.WithGlobalPoolId(defaultPoolId)
		isNeedUpdate = true
	}

	// Compare headers: entity stores as comma-joined *string, spec stores as []string.
	// Normalize both to lowercase []string for order-independent comparison.
	decodeHeaders := func(h *string) []string {
		if h == nil || *h == "" {
			return []string{}
		}
		parts := strings.Split(*h, ",")
		normalized := make([]string, 0, len(parts))
		for _, p := range parts {
			normalized = append(normalized, strings.ToLower(p))
		}
		return normalized
	}

	currentHeaders := decodeHeaders(currentListener.Headers)
	specHeaders := make([]string, 0, len(listenerSpec.Headers))
	for _, h := range listenerSpec.Headers {
		specHeaders = append(specHeaders, strings.ToLower(h))
	}

	if !stringSlicesEqualUnordered(currentHeaders, specHeaders) {
		message = append(message, fmt.Sprintf("headers (%v -> %v)", currentHeaders, specHeaders))
		if len(specHeaders) > 0 {
			joined := strings.Join(specHeaders, ",")
			updateOptions.Headers = &joined
		} else {
			updateOptions.Headers = nil
		}
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil, nil
	}
	return updateOptions, message, nil
}
