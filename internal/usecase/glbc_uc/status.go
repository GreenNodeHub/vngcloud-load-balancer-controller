package glbc_uc

import (
	"context"
	"slices"

	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) statusAddListener(ctx context.Context, listenerId string, port int) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) bool {
		// check on fresh copy if already exists with same values
		for _, l := range obj.Status.CreatedListeners {
			if l.Id == listenerId && l.Port == port {
				return false // no change needed
			}
		}
		for i := range obj.Status.CreatedListeners {
			if obj.Status.CreatedListeners[i].Id == listenerId {
				obj.Status.CreatedListeners[i].Port = port
				return true
			}
		}
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedGlobalListener{Id: listenerId, Port: port})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddPool(ctx context.Context, poolId string, name string) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) bool {
		// check on fresh copy if already exists with same values
		for _, p := range obj.Status.CreatedPools {
			if p.Id == poolId && p.Name == name {
				return false // no change needed
			}
		}
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				return true
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedGlobalPool{Id: poolId, Name: name, CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{}})
		return true
	})
}

func (t *defaultModelDeployTask) statusUpdatePoolMember(ctx context.Context, poolId string, name string, poolMembers []v1alpha1.CreatedGlobalPoolMember) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) bool {
		// check on fresh copy if already exists with same values
		for _, p := range obj.Status.CreatedPools {
			if p.Id == poolId && p.Name == name && createdGlobalPoolMembersEqual(p.CreatedPoolMembers, poolMembers) {
				return false // no change needed
			}
		}
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				obj.Status.CreatedPools[i].CreatedPoolMembers = poolMembers
				return true
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedGlobalPool{Id: poolId, Name: name, CreatedPoolMembers: poolMembers})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddLoadBalancerId(ctx context.Context, lbId *string, vips []v1alpha1.GlobalLoadBalancerVIPStatus, domains []string) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) bool {
		// check on fresh copy if already equal (order-independent)
		if ptr.Equal(obj.Status.LoadBalancerId, lbId) &&
			globalLoadBalancerVIPsEqual(obj.Status.Vips, vips) &&
			stringSlicesEqualUnordered(obj.Status.Domains, domains) {
			return false // no change needed
		}
		obj.Status.LoadBalancerId = lbId
		obj.Status.Vips = vips
		obj.Status.Domains = domains
		return true
	})
}

// ============================================================================
// Equality helper functions for status comparisons (order-independent)
// ============================================================================

// globalLoadBalancerVIPsEqual compares two slices of GlobalLoadBalancerVIPStatus (order-independent)
func globalLoadBalancerVIPsEqual(a, b []v1alpha1.GlobalLoadBalancerVIPStatus) bool {
	if len(a) != len(b) {
		return false
	}
	for _, vipA := range a {
		if !slices.ContainsFunc(b, vipA.Equal) {
			return false
		}
	}
	return true
}

// stringSlicesEqualUnordered compares two string slices (order-independent)
func stringSlicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, s := range a {
		if !slices.Contains(b, s) {
			return false
		}
	}
	return true
}

// createdGlobalPoolsEqual compares two slices of CreatedGlobalPool (order-independent)
func createdGlobalPoolsEqual(a, b []v1alpha1.CreatedGlobalPool) bool {
	if len(a) != len(b) {
		return false
	}
	for _, poolA := range a {
		if !slices.ContainsFunc(b, poolA.Equal) {
			return false
		}
	}
	return true
}

// createdGlobalListenersEqual compares two slices of CreatedGlobalListener (order-independent)
func createdGlobalListenersEqual(a, b []v1alpha1.CreatedGlobalListener) bool {
	if len(a) != len(b) {
		return false
	}
	for _, listenerA := range a {
		if !slices.ContainsFunc(b, listenerA.Equal) {
			return false
		}
	}
	return true
}

// createdGlobalPoolMembersEqual compares two slices of CreatedGlobalPoolMember (order-independent)
func createdGlobalPoolMembersEqual(a, b []v1alpha1.CreatedGlobalPoolMember) bool {
	if len(a) != len(b) {
		return false
	}
	for _, memberA := range a {
		if !slices.ContainsFunc(b, memberA.Equal) {
			return false
		}
	}
	return true
}
