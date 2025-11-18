package lbc_uc

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)

// ============================================================================
// LAYER 1: Validate the LoadBalancerConfig itself (internal consistency)
// These validations run before we have a load balancer ID
// ============================================================================

// validateSelf validates the LoadBalancerConfig itself for internal consistency
// This is the first validation layer that checks the spec without external dependencies
func (t *defaultModelDeployTask) validateSelf(ctx context.Context) error {
	// Run all self-validation checks
	if err := t.validateSelfListenerPorts(ctx); err != nil {
		return err
	}

	// Add more self-validations here in the future:
	// if err := t.validateSelfPoolMembers(ctx); err != nil {
	//     return err
	// }

	return nil
}

// validateSelfListenerPorts checks for duplicate ports within this LBC's spec
func (t *defaultModelDeployTask) validateSelfListenerPorts(_ context.Context) error {
	portToProtocolMap := make(map[int32]string)
	for _, listener := range t.lbConfig.Spec.Listeners {
		if existingProtocol, exists := portToProtocolMap[listener.ProtocolPort]; exists {
			// Found duplicate port - return error
			return errs.NewNoNeedRequeue(fmt.Sprintf("duplicate listener port %d in LoadBalancerConfig %s/%s: cannot have both %s and %s listeners on the same port (VNGCloud limitation)",
				listener.ProtocolPort, t.lbConfig.Namespace, t.lbConfig.Name, existingProtocol, listener.Protocol))
		}
		portToProtocolMap[listener.ProtocolPort] = string(listener.Protocol)
	}
	return nil
}

// ============================================================================
// LAYER 2: Validate across LoadBalancerConfigs sharing the same load balancer
// These validations run after we have the load balancer ID
// ============================================================================

// validateCrossLBCs validates this LBC against other LBCs that share the same load balancer
// This is the second validation layer that checks for conflicts across multiple LBCs
// Note: LBCs can be in different namespaces but share the same load balancer ID
func (t *defaultModelDeployTask) validateCrossLBCs(ctx context.Context, lbId string) error {
	// List ALL LoadBalancerConfigs across ALL namespaces to find those sharing the same LB ID
	allLBCs := &v1alpha1.LoadBalancerConfigList{}
	if err := t.k8sRepo.ListLoadBalancerConfig(ctx, allLBCs); err != nil {
		return errors.Wrap(err, "failed to list LoadBalancerConfigs for cross-validation")
	}

	// Run all cross-validation checks
	if err := t.validateCrossListenerPorts(ctx, lbId, allLBCs); err != nil {
		return err
	}

	// Validate that listeners on the same port have the same default pool
	if err := t.validateCrossListenerDefaultPools(ctx, lbId, allLBCs); err != nil {
		return err
	}

	// Add more cross-validations here in the future:
	// if err := t.validateCrossPoolMembers(ctx, lbId, allLBCs); err != nil {
	//     return err
	// }

	return nil
}

// validateCrossListenerPorts validates that each port only has one protocol across all LBCs sharing the same load balancer
// VNGCloud doesn't support multiple protocols on the same port
func (t *defaultModelDeployTask) validateCrossListenerPorts(_ context.Context, lbId string, allLBCs *v1alpha1.LoadBalancerConfigList) error {
	// Map: port -> protocol -> list of LBCs using this port+protocol combination
	type protocolInfo struct {
		protocol loadbalancerv2.ListenerProtocol
		lbcName  string
	}
	portToProtocolMap := make(map[int32][]protocolInfo)

	// Collect all listeners from ALL LBCs (including current one) that share this load balancer
	for _, lbc := range allLBCs.Items {
		// Determine which load balancer this LBC references
		lbcId := ""
		if lbc.Spec.LoadBalancerId != nil && *lbc.Spec.LoadBalancerId != "" {
			lbcId = *lbc.Spec.LoadBalancerId
		} else if lbc.Status.LoadBalancerId != nil && *lbc.Status.LoadBalancerId != "" {
			lbcId = *lbc.Status.LoadBalancerId
		}

		// Skip if this LBC uses a different load balancer
		if lbcId == "" || lbcId != lbId {
			continue
		}

		// Collect all listeners from this LBC
		for _, listener := range lbc.Spec.Listeners {
			portToProtocolMap[listener.ProtocolPort] = append(portToProtocolMap[listener.ProtocolPort], protocolInfo{
				protocol: listener.Protocol,
				lbcName:  fmt.Sprintf("%s/%s", lbc.Namespace, lbc.Name),
			})
		}
	}

	// Validate that each port only has one protocol
	for port, protocolInfos := range portToProtocolMap {
		if len(protocolInfos) <= 1 {
			continue
		}

		// Check if all protocols are the same
		firstProtocol := protocolInfos[0].protocol
		for _, info := range protocolInfos[1:] {
			if info.protocol != firstProtocol {
				return errs.NewNoNeedRequeue(fmt.Sprintf("port %d has multiple protocols on load balancer %s: %s uses %s, %s uses %s. VNGCloud does not support multiple protocols on the same port",
					port, lbId, protocolInfos[0].lbcName, firstProtocol, info.lbcName, info.protocol))
			}
		}
	}

	return nil
}

// validateCrossListenerDefaultPools checks that listeners on the same port across LBCs have the same default pool
// Multiple LBCs can share the same load balancer ID, but each unique listener port must have consistent default pool configuration
func (t *defaultModelDeployTask) validateCrossListenerDefaultPools(_ context.Context, lbId string, allLBCs *v1alpha1.LoadBalancerConfigList) error {
	// Map: port -> list of pool info from all LBCs
	type poolInfo struct {
		poolName string
		lbcName  string
	}
	portToPoolMap := make(map[int32][]poolInfo)

	// Collect all listener port -> default pool mappings from ALL LBCs sharing this load balancer
	for _, lbc := range allLBCs.Items {
		// Determine which load balancer this LBC references
		lbcId := ""
		if lbc.Spec.LoadBalancerId != nil && *lbc.Spec.LoadBalancerId != "" {
			lbcId = *lbc.Spec.LoadBalancerId
		} else if lbc.Status.LoadBalancerId != nil && *lbc.Status.LoadBalancerId != "" {
			lbcId = *lbc.Status.LoadBalancerId
		}

		// Skip if this LBC uses a different load balancer
		if lbcId == "" || lbcId != lbId {
			continue
		}

		// Collect default pool for each listener port
		// Note: nil DefaultPoolName means "no default pool" and is represented as empty string
		for _, listener := range lbc.Spec.Listeners {
			defaultPool := ""
			if listener.DefaultPoolName != nil {
				defaultPool = *listener.DefaultPoolName
			}

			portToPoolMap[listener.ProtocolPort] = append(portToPoolMap[listener.ProtocolPort], poolInfo{
				poolName: defaultPool, // empty string means no default pool (nil pointer)
				lbcName:  fmt.Sprintf("%s/%s", lbc.Namespace, lbc.Name),
			})
		}
	}

	// Validate that each port has consistent default pools across all LBCs
	// This includes the case where nil (no default pool) must be consistent too
	for port, poolInfos := range portToPoolMap {
		if len(poolInfos) <= 1 {
			continue
		}

		// Check if all default pools are the same (including empty string for nil/no default pool)
		firstPool := poolInfos[0].poolName
		for _, info := range poolInfos[1:] {
			if info.poolName != firstPool {
				firstPoolDisplay := firstPool
				if firstPoolDisplay == "" {
					firstPoolDisplay = "<no default pool>"
				}
				infoPoolDisplay := info.poolName
				if infoPoolDisplay == "" {
					infoPoolDisplay = "<no default pool>"
				}
				return errs.NewNoNeedRequeue(fmt.Sprintf("port %d has different default pools on load balancer %s: %s uses '%s', %s uses '%s'. All listeners on the same port must have the same default pool when sharing a load balancer",
					port, lbId, poolInfos[0].lbcName, firstPoolDisplay, info.lbcName, infoPoolDisplay))
			}
		}
	}

	return nil
}
