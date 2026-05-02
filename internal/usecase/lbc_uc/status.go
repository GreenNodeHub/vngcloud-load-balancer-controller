package lbc_uc

import (
	"context"
	"maps"
	"slices"

	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) statusAddListener(ctx context.Context, listenerId string, port int) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
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
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedListener{Id: listenerId, Port: port})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddPolicy(ctx context.Context, listenerId string, port int, policyId string) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		// check on fresh copy if already exists with same values
		for _, l := range obj.Status.CreatedListeners {
			if l.Id == listenerId && l.Port == port {
				for _, p := range l.CreatedPolicies {
					if p.Id == policyId {
						return false // no change needed
					}
				}
			}
		}
		for i := range obj.Status.CreatedListeners {
			if obj.Status.CreatedListeners[i].Id == listenerId {
				obj.Status.CreatedListeners[i].Port = port
				for j := range obj.Status.CreatedListeners[i].CreatedPolicies {
					if obj.Status.CreatedListeners[i].CreatedPolicies[j].Id == policyId {
						return false
					}
				}
				obj.Status.CreatedListeners[i].CreatedPolicies = append(obj.Status.CreatedListeners[i].CreatedPolicies, v1alpha1.CreatedPolicy{Id: policyId})
				return true
			}
		}
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedListener{Id: listenerId, Port: port, CreatedPolicies: []v1alpha1.CreatedPolicy{{Id: policyId}}})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddPool(ctx context.Context, poolId string, name string) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
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
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedPool{Id: poolId, Name: name})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddPoolMember(ctx context.Context, poolId string, name string, members []v1alpha1.PoolMember) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		// check on fresh copy if already exists with same values
		for _, p := range obj.Status.CreatedPools {
			if p.Id == poolId && p.Name == name && t.comparePoolMembers(ctx, p.CreatedMembers, members) {
				return false // no change needed
			}
		}
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				obj.Status.CreatedPools[i].CreatedMembers = members
				return true
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedPool{Id: poolId, Name: name, CreatedMembers: members})
		return true
	})
}

func (t *defaultModelDeployTask) statusAddLoadBalancerId(ctx context.Context, lbId *string, address *string) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		// check on fresh copy if already equal
		if ptr.Equal(obj.Status.LoadBalancerId, lbId) && ptr.Equal(obj.Status.Address, address) {
			return false // no change needed
		}
		obj.Status.LoadBalancerId = lbId
		obj.Status.Address = address
		return true
	})
}

func (t *defaultModelDeployTask) statusAddCreatedTags(ctx context.Context, tags map[string]string) error {
	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		// check on fresh copy if already equal
		if maps.Equal(obj.Status.CreatedTags, tags) {
			return false // no change needed
		}
		obj.Status.CreatedTags = tags
		return true
	})
}

// ============================================================================
// Equality helper functions for status comparisons (order-independent)
// ============================================================================

// createdPoolsEqual compares two slices of CreatedPool (order-independent)
func createdPoolsEqual(a, b []v1alpha1.CreatedPool) bool {
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

// createdListenersEqual compares two slices of CreatedListener (order-independent)
func createdListenersEqual(a, b []v1alpha1.CreatedListener) bool {
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

// createdCertificatesEqual compares two slices of CreatedCertificate (order-independent)
func createdCertificatesEqual(a, b []v1alpha1.CreatedCertificate) bool {
	if len(a) != len(b) {
		return false
	}
	for _, certA := range a {
		if !slices.ContainsFunc(b, certA.Equal) {
			return false
		}
	}
	return true
}
