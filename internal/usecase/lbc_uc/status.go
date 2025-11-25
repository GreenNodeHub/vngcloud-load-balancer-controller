package lbc_uc

import (
	"context"
	"maps"

	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) statusAddListener(ctx context.Context, listenerId string, port int) error {
	// check if already exists with same values
	for _, l := range t.lbConfig.Status.CreatedListeners {
		if l.Id == listenerId && l.Port == port {
			return nil // no change needed
		}
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		for i := range obj.Status.CreatedListeners {
			if obj.Status.CreatedListeners[i].Id == listenerId {
				obj.Status.CreatedListeners[i].Port = port
				return
			}
		}
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedListener{Id: listenerId, Port: port})
	})
}

func (t *defaultModelDeployTask) statusAddPolicy(ctx context.Context, listenerId string, port int, policyId string) error {
	// check if already exists with same values
	for _, l := range t.lbConfig.Status.CreatedListeners {
		if l.Id == listenerId && l.Port == port {
			for _, p := range l.CreatedPolicies {
				if p.Id == policyId {
					return nil // no change needed
				}
			}
		}
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		for i := range obj.Status.CreatedListeners {
			if obj.Status.CreatedListeners[i].Id == listenerId {
				obj.Status.CreatedListeners[i].Port = port
				for j := range obj.Status.CreatedListeners[i].CreatedPolicies {
					if obj.Status.CreatedListeners[i].CreatedPolicies[j].Id == policyId {
						return
					}
				}
				obj.Status.CreatedListeners[i].CreatedPolicies = append(obj.Status.CreatedListeners[i].CreatedPolicies, v1alpha1.CreatedPolicy{Id: policyId})
				return
			}
		}
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedListener{Id: listenerId, Port: port, CreatedPolicies: []v1alpha1.CreatedPolicy{{Id: policyId}}})
	})
}

func (t *defaultModelDeployTask) statusAddPool(ctx context.Context, poolId string, name string) error {
	// check if already exists with same values
	for _, p := range t.lbConfig.Status.CreatedPools {
		if p.Id == poolId && p.Name == name {
			return nil // no change needed
		}
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				return
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedPool{Id: poolId, Name: name})
	})
}

func (t *defaultModelDeployTask) statusAddPoolMember(ctx context.Context, poolId string, name string, members []v1alpha1.PoolMember) error {
	// check if already exists with same values
	for _, p := range t.lbConfig.Status.CreatedPools {
		if p.Id == poolId && p.Name == name && t.comparePoolMembers(ctx, p.CreatedMembers, members) {
			return nil // no change needed
		}
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				obj.Status.CreatedPools[i].CreatedMembers = members
				return
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedPool{Id: poolId, Name: name, CreatedMembers: members})
	})
}

func (t *defaultModelDeployTask) statusAddLoadBalancerId(ctx context.Context, lbId *string, address *string) error {
	// check if already equal
	if ptr.Equal(t.lbConfig.Status.LoadBalancerId, lbId) && ptr.Equal(t.lbConfig.Status.Address, address) {
		return nil // no change needed
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.LoadBalancerId = lbId
		obj.Status.Address = address
	})
}

func (t *defaultModelDeployTask) statusAddCreatedTags(ctx context.Context, tags map[string]string) error {
	// check if already equal
	if maps.Equal(t.lbConfig.Status.CreatedTags, tags) {
		return nil // no change needed
	}

	return t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) {
		obj.Status.CreatedTags = tags
	})
}
