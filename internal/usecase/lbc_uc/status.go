package lbc_uc

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) statusAddListener(ctx context.Context, listenerId string, port int) error {
	// A cloud resource we cannot name is a cloud resource we have lost: nothing later can
	// find it, update it or delete it. Recording it with an empty id is worse than failing
	// here, because id is the key of a map-list - the API server rejects the whole status
	// patch, every reconcile, and the LBC never moves again. deployLoadBalancer already
	// guards the load balancer's own id this way.
	if listenerId == "" {
		return errors.New("listener has no id after create, need to retry")
	}

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
	// A cloud resource we cannot name is a cloud resource we have lost: nothing later can
	// find it, update it or delete it. Recording it with an empty id is worse than failing
	// here, because id is the key of a map-list - the API server rejects the whole status
	// patch, every reconcile, and the LBC never moves again. deployLoadBalancer already
	// guards the load balancer's own id this way.
	if policyId == "" {
		return errors.New("policy has no id after create, need to retry")
	}

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
	// A cloud resource we cannot name is a cloud resource we have lost: nothing later can
	// find it, update it or delete it. Recording it with an empty id is worse than failing
	// here, because id is the key of a map-list - the API server rejects the whole status
	// patch, every reconcile, and the LBC never moves again. deployLoadBalancer already
	// guards the load balancer's own id this way.
	if poolId == "" {
		return errors.New("pool has no id after create, need to retry")
	}

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

// statusSetCreatedLoadBalancerId records that this LBC created the load balancer, which is
// what makes it the controller's to delete later. See createdByThisCluster.
func (t *defaultModelDeployTask) statusSetCreatedLoadBalancerId(ctx context.Context, lbId string) error {
	err := t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		if obj.Status.CreatedLoadBalancerId != nil && *obj.Status.CreatedLoadBalancerId == lbId {
			return false // no change needed
		}
		obj.Status.CreatedLoadBalancerId = &lbId
		return true
	})
	if err != nil {
		return err
	}

	// The patch helper deliberately mutates a fresh copy rather than the object it was given,
	// so bring ours up to date by hand: deployTags, later in this same reconcile, decides from
	// it whether to record provenance on the load balancer. Without this the tag would have to
	// wait for a reconcile that may not come before the LBC is deleted.
	t.lbConfig.Status.CreatedLoadBalancerId = &lbId
	return nil
}

// statusSetAdoptedLoadBalancerId records that this LBC adopted the load balancer - reached
// through an annotation, not created by the controller - which is what makes it never this
// cluster's to delete, even if the pin annotation is later removed. See createdByThisCluster.
func (t *defaultModelDeployTask) statusSetAdoptedLoadBalancerId(ctx context.Context, lbId string) error {
	err := t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		if obj.Status.AdoptedLoadBalancerId != nil && *obj.Status.AdoptedLoadBalancerId == lbId {
			return false // no change needed
		}
		obj.Status.AdoptedLoadBalancerId = &lbId
		return true
	})
	if err != nil {
		return err
	}
	// The patch helper mutates a fresh copy, never the object it was given, so keep the
	// in-memory copy honest for the rest of this reconcile.
	t.lbConfig.Status.AdoptedLoadBalancerId = &lbId
	return nil
}

// statusSetRetiringLoadBalancer parks the record of the load balancer being migrated away
// from - its id and everything this LBC created on it - so the old load balancer can keep
// serving until the new one is fully deployed, and still be torn down afterwards even
// across a controller restart. Passing nil clears the field once the teardown is done.
func (t *defaultModelDeployTask) statusSetRetiringLoadBalancer(ctx context.Context, retiring *v1alpha1.RetiringLoadBalancer) error {
	err := t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		if obj.Status.RetiringLoadBalancer == nil && retiring == nil {
			return false // no change needed
		}
		obj.Status.RetiringLoadBalancer = retiring
		return true
	})
	if err != nil {
		return err
	}
	t.lbConfig.Status.RetiringLoadBalancer = retiring
	return nil
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

// statusClearCreatedResources forgets the listeners and pools recorded in status. Used
// when the config moves to another load balancer: entries left behind describe
// resources on the old one, and their names would collide with the new one's.
func (t *defaultModelDeployTask) statusClearCreatedResources(ctx context.Context) error {
	err := t.k8sRepo.PatchMutateStatusLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.LoadBalancerConfig) bool {
		if len(obj.Status.CreatedListeners) == 0 && len(obj.Status.CreatedPools) == 0 &&
			len(obj.Status.CreatedTags) == 0 {
			return false // no change needed
		}
		obj.Status.CreatedListeners = nil
		obj.Status.CreatedPools = nil
		// createdTags too: it records what was written on the load balancer being left behind,
		// and buildTag treats those keys as this cluster's to drop. Carried into the next load
		// balancer it would take keys off that one which were never written by us.
		obj.Status.CreatedTags = nil
		return true
	})
	if err != nil {
		return err
	}
	// The patch helper mutates a fresh copy, never the object it was given. A migration
	// continues in this same reconcile into deployTags on the NEW load balancer, and a stale
	// in-memory createdTags there would be treated as this cluster's keys to drop - deleting
	// tags on the new load balancer that were never ours, including another cluster's
	// provenance tag.
	t.lbConfig.Status.CreatedListeners = nil
	t.lbConfig.Status.CreatedPools = nil
	t.lbConfig.Status.CreatedTags = nil
	return nil
}
