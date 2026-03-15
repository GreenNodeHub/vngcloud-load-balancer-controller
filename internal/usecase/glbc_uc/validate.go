package glbc_uc

import (
	"context"
)

// ============================================================================
// LAYER 1: Validate the GlobalLoadBalancerConfig itself (internal consistency)
// These validations run before we have a load balancer ID
// ============================================================================

func (t *defaultModelDeployTask) validateSelf(_ context.Context) error {

	return nil
}

// ============================================================================
// LAYER 2: Validate across GlobalLoadBalancerConfigs sharing the same load balancer
// These validations run after we have the load balancer ID
// ============================================================================

func (t *defaultModelDeployTask) validateCrossGLBCs(_ context.Context, lbId string) error {

	return nil
}
