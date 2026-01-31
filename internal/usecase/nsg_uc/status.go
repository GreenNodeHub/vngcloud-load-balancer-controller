package nsg_uc

import (
	"context"
	"slices"

	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (uc *nsgUseCase) statusSetSelectedNodes(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, nodeInfos []v1alpha1.NodeInfo) error {
	return uc.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject, func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) bool {
		// check on fresh copy if already equal
		if nodeInfosEqual(obj.Status.SelectedNodes, nodeInfos) {
			return false // no change needed
		}
		obj.Status.SelectedNodes = nodeInfos
		return true
	})
}

func (m *nsgUseCase) statusAddStatusManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, secgroupID *string, err error) error {
	errStr := errorToStringPtr(err)
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) bool {
			// check on fresh copy if already equal
			if ptr.Equal(obj.Status.ManagedSecurityGroup.Id, secgroupID) &&
				ptr.Equal(obj.Status.ManagedSecurityGroup.Error, errStr) {
				return false // no change needed
			}
			obj.Status.ManagedSecurityGroup.Id = secgroupID
			obj.Status.ManagedSecurityGroup.Error = errStr
			return true
		})
}

// update the status.serverSecurityGroups of nsgObject for a specific server
func (m *nsgUseCase) statusUpdateNodeSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, serverId string, err error, attachedSecgroupIds []string) error {
	errStr := errorToStringPtr(err)
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) bool {
			// check on fresh copy if already equal
			for _, serverSecgroup := range obj.Status.ServerSecurityGroups {
				if serverSecgroup.ServerId == serverId {
					if v1alpha1.StringSlicesEqualUnordered(serverSecgroup.AttachedSecurityGroupIds, attachedSecgroupIds) &&
						ptr.Equal(serverSecgroup.Error, errStr) {
						return false // no change needed
					}
					break
				}
			}

			// Create the new status for this server
			newStatus := v1alpha1.ServerSecurityGroupStatus{
				ServerId:                 serverId,
				AttachedSecurityGroupIds: attachedSecgroupIds,
				Error:                    errStr,
			}

			// Find and update the existing status, or append if not found
			for i, serverSecgroup := range obj.Status.ServerSecurityGroups {
				if serverSecgroup.ServerId == serverId {
					obj.Status.ServerSecurityGroups[i] = newStatus
					return true
				}
			}

			// Server not found in status, append new entry
			obj.Status.ServerSecurityGroups = append(obj.Status.ServerSecurityGroups, newStatus)
			return true
		})
}

func (m *nsgUseCase) statusServerSecurityGroupStatus(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, ssgs []v1alpha1.ServerSecurityGroupStatus) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) bool {
			// check on fresh copy if already equal
			if serverSecurityGroupStatusesEqual(obj.Status.ServerSecurityGroups, ssgs) {
				return false // no change needed
			}
			obj.Status.ServerSecurityGroups = ssgs
			return true
		})
}

// nodeInfosEqual compares two slices of NodeInfo (order-independent)
func nodeInfosEqual(a, b []v1alpha1.NodeInfo) bool {
	if len(a) != len(b) {
		return false
	}
	// Check that every item in a exists in b
	for _, itemA := range a {
		if !slices.ContainsFunc(b, itemA.Equal) {
			return false
		}
	}
	return true
}

// serverSecurityGroupStatusesEqual compares two slices of ServerSecurityGroupStatus (order-independent)
func serverSecurityGroupStatusesEqual(a, b []v1alpha1.ServerSecurityGroupStatus) bool {
	if len(a) != len(b) {
		return false
	}
	// Check that every item in a exists in b
	for _, itemA := range a {
		if !slices.ContainsFunc(b, itemA.Equal) {
			return false
		}
	}
	return true
}
