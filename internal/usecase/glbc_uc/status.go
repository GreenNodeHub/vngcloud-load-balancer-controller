package glbc_uc

import (
	"context"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) statusAddListener(ctx context.Context, listenerId string, port int) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		// check if already exist
		for i := range obj.Status.CreatedListeners {
			if obj.Status.CreatedListeners[i].Id == listenerId {
				obj.Status.CreatedListeners[i].Port = port
				return
			}
		}
		obj.Status.CreatedListeners = append(obj.Status.CreatedListeners, v1alpha1.CreatedGlobalListener{Id: listenerId, Port: port})
	})
}

func (t *defaultModelDeployTask) statusAddPool(ctx context.Context, poolId string, name string) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		// check if already exist
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				return
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedGlobalPool{Id: poolId, Name: name, CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{}})
	})
}

func (t *defaultModelDeployTask) statusUpdatePoolMember(ctx context.Context, poolId string, name string, poolMembers []v1alpha1.CreatedGlobalPoolMember) error {
	return t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, t.lbConfig, func(ctx context.Context, obj *v1alpha1.GlobalLoadBalancerConfig) {
		// check if already exist
		for i := range obj.Status.CreatedPools {
			if obj.Status.CreatedPools[i].Id == poolId {
				obj.Status.CreatedPools[i].Name = name
				obj.Status.CreatedPools[i].CreatedPoolMembers = poolMembers
				return
			}
		}
		obj.Status.CreatedPools = append(obj.Status.CreatedPools, v1alpha1.CreatedGlobalPool{Id: poolId, Name: name, CreatedPoolMembers: poolMembers})
	})
}
