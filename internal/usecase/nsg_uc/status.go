package nsg_uc

import (
	"context"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (uc *nsgUseCase) statusSetSelectedNodes(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, nodeInfos []v1alpha1.NodeInfo) error {
	return uc.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject, func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
		obj.Status.SelectedNodes = nodeInfos
	})
}

func (m *nsgUseCase) statusAddStatusManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, secgroupID string, err error) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
			if secgroupID == "" {
				obj.Status.ManagedSecurityGroup.Id = nil
			} else {
				obj.Status.ManagedSecurityGroup.Id = &secgroupID
			}
			obj.Status.ManagedSecurityGroup.Error = errorToStringPtr(err)
		})
}

// update the status.serverSecurityGroups of nsgObject for a specific server
func (m *nsgUseCase) statusUpdateNodeSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, serverId string, err error, attachedSecgroupIds []string) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
			// Create the new status for this server
			newStatus := v1alpha1.ServerSecurityGroupStatus{
				ServerId:                 serverId,
				AttachedSecurityGroupIds: attachedSecgroupIds,
				Error:                    errorToStringPtr(err),
			}

			// Find and update the existing status, or append if not found
			for i, serverSecgroup := range obj.Status.ServerSecurityGroups {
				if serverSecgroup.ServerId == serverId {
					obj.Status.ServerSecurityGroups[i] = newStatus
					return
				}
			}

			// Server not found in status, append new entry
			obj.Status.ServerSecurityGroups = append(obj.Status.ServerSecurityGroups, newStatus)
		})
}

func (m *nsgUseCase) statusServerSecurityGroupStatus(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, ssgs []v1alpha1.ServerSecurityGroupStatus) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
			obj.Status.ServerSecurityGroups = ssgs
		})
}
