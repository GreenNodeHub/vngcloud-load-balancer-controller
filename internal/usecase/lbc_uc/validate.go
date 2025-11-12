package lbc_uc

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

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

	// // Run all cross-validation checks
	// if err := t.validateCrossListenerPorts(ctx, lbId, allLBCs); err != nil {
	// 	return err
	// }

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

// validateCrossListenerPorts checks for port conflicts across all LBCs sharing the same load balancer
// VNGCloud doesn't support multiple listeners on the same port, even with different protocols
func (t *defaultModelDeployTask) validateCrossListenerPorts(_ context.Context, lbId string, allLBCs *v1alpha1.LoadBalancerConfigList) error {
	// Collect all listener ports from other LBCs that share this load balancer
	portToLBCMap := make(map[int32]string) // port -> LBC name that owns it

	for _, lbc := range allLBCs.Items {
		// Skip the current LBC we're deploying
		if lbc.Name == t.lbConfig.Name && lbc.Namespace == t.lbConfig.Namespace {
			continue
		}

		// Determine which load balancer this LBC references
		lbcId := ""
		if lbc.Status.LoadBalancerId != nil {
			lbcId = *lbc.Status.LoadBalancerId
		} else if lbc.Spec.LoadBalancerId != nil {
			lbcId = *lbc.Spec.LoadBalancerId
		}

		// Skip if this LBC uses a different load balancer
		if lbcId == "" || lbcId != lbId {
			continue
		}

		// Check each listener in this LBC for port conflicts
		for _, listener := range lbc.Spec.Listeners {
			if existingLBC, exists := portToLBCMap[listener.ProtocolPort]; exists {
				// Multiple other LBCs using same port - shouldn't happen but log it
				t.logger.Warnf("Port %d is used by multiple LoadBalancerConfigs: %s and %s",
					listener.ProtocolPort, existingLBC, lbc.Name)
				continue
			}
			portToLBCMap[listener.ProtocolPort] = fmt.Sprintf("%s/%s", lbc.Namespace, lbc.Name)
		}
	}

	// 3. Check if any of our listeners conflict with existing listeners
	for _, listener := range t.lbConfig.Spec.Listeners {
		if existingLBC, exists := portToLBCMap[listener.ProtocolPort]; exists {
			return fmt.Errorf("port %d is already in use by LoadBalancerConfig %s on load balancer %s, cannot use in %s/%s",
				listener.ProtocolPort, existingLBC, lbId, t.lbConfig.Namespace, t.lbConfig.Name)
		}
	}

	return nil
}

// validateCrossListenerDefaultPools checks that listeners on the same port across LBCs have the same default pool
// Multiple LBCs can share the same load balancer ID, but each unique listener port must have consistent default pool configuration
func (t *defaultModelDeployTask) validateCrossListenerDefaultPools(_ context.Context, lbId string, allLBCs *v1alpha1.LoadBalancerConfigList) error {
	// Map: port -> default pool name (and which LBC defined it)
	type poolInfo struct {
		poolName string
		lbcName  string
	}
	portToPoolMap := make(map[int32]poolInfo)

	// First, collect all listener port -> default pool mappings from other LBCs sharing this load balancer
	for _, lbc := range allLBCs.Items {
		// Skip the current LBC we're deploying
		if lbc.Name == t.lbConfig.Name && lbc.Namespace == t.lbConfig.Namespace {
			continue
		}

		// Determine which load balancer this LBC references
		lbcId := ""
		if lbc.Status.LoadBalancerId != nil {
			lbcId = *lbc.Status.LoadBalancerId
		} else if lbc.Spec.LoadBalancerId != nil {
			lbcId = *lbc.Spec.LoadBalancerId
		}

		// Skip if this LBC uses a different load balancer
		if lbcId == "" || lbcId != lbId {
			continue
		}

		// Collect default pool for each listener port
		for _, listener := range lbc.Spec.Listeners {
			defaultPool := ""
			if listener.DefaultPoolName != nil {
				defaultPool = *listener.DefaultPoolName
			}

			if existing, exists := portToPoolMap[listener.ProtocolPort]; exists {
				// Port already seen in another LBC - verify the pool matches
				if existing.poolName != defaultPool {
					t.logger.Warnf("Port %d has conflicting default pools across LoadBalancerConfigs: %s uses '%s', %s uses '%s'",
						listener.ProtocolPort, existing.lbcName, existing.poolName, fmt.Sprintf("%s/%s", lbc.Namespace, lbc.Name), defaultPool)
				}
			} else {
				// First time seeing this port
				portToPoolMap[listener.ProtocolPort] = poolInfo{
					poolName: defaultPool,
					lbcName:  fmt.Sprintf("%s/%s", lbc.Namespace, lbc.Name),
				}
			}
		}
	}

	// Now check if any of our listeners conflict with existing mappings
	for _, listener := range t.lbConfig.Spec.Listeners {
		defaultPool := ""
		if listener.DefaultPoolName != nil {
			defaultPool = *listener.DefaultPoolName
		}

		if existing, exists := portToPoolMap[listener.ProtocolPort]; exists {
			if existing.poolName != defaultPool {
				return errs.NewNoNeedRequeue(fmt.Sprintf("listener port %d in LoadBalancerConfig %s/%s has default pool '%s', but LoadBalancerConfig %s already uses default pool '%s' for the same port on load balancer %s. All listeners on the same port must have the same default pool when sharing a load balancer",
					listener.ProtocolPort, t.lbConfig.Namespace, t.lbConfig.Name, defaultPool, existing.lbcName, existing.poolName, lbId))
			}
		}
	}

	return nil
}
