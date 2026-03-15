package glbc_uc

import (
	"context"
	"fmt"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) deployPoolMembers(ctx context.Context, lbId, poolId, poolName string, poolMembersSpec []v1alpha1.GlobalPoolMember) ([]v1alpha1.CreatedGlobalPoolMember, error) {
	currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)
	if err != nil {
		t.logger.Error("Failed to get pool members: ", err)
		return nil, err
	}

	patchRequest, allActionMessages := t.buildPatchGlobalPoolMemberRequest(ctx, lbId, poolId, currentPoolMembers, poolMembersSpec)
	if patchRequest != nil {
		t.logger.Info("Need to update pool members: ", strings.Join(allActionMessages, "; "))
		if err := t.vngcloudRepo.PatchGlobalPoolMembers(ctx, lbId, poolId, patchRequest); err != nil {
			t.logger.Error("Failed to patch pool members: ", err)
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			return nil, err
		}

		currentPoolMembers, err = t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)
		if err != nil {
			t.logger.Error("Failed to get pool members after patching: ", err)
			return nil, err
		}
	}

	searchPoolMemberByName := func(name string) *entityv2.GlobalPoolMember {
		for _, p := range currentPoolMembers.Items {
			if p.Name == name {
				return p
			}
		}
		return nil
	}

	createdPoolMembers := make([]v1alpha1.CreatedGlobalPoolMember, 0)
	for _, poolMemberSpec := range poolMembersSpec {
		if currentPoolMember := searchPoolMemberByName(poolMemberSpec.Name); currentPoolMember != nil {
			createdMembers := make([]v1alpha1.GlobalMember, 0)
			for _, member := range currentPoolMember.Members.Items {
				createdMembers = append(createdMembers, v1alpha1.GlobalMember{
					Name:        member.Name,
					Address:     member.Address,
					Port:        member.Port,
					BackupRole:  member.BackupRole,
					Weight:      &member.Weight,
					MonitorPort: &member.MonitorPort,
					SubnetID:    member.SubnetID,
				})
			}
			createdPoolMembers = append(createdPoolMembers, v1alpha1.CreatedGlobalPoolMember{
				Id:             currentPoolMember.ID,
				Name:           currentPoolMember.Name,
				CreatedMembers: createdMembers,
			})
		} else {
			return nil, fmt.Errorf("pool member %s not found after deployment", poolMemberSpec.Name)
		}
	}

	// update status
	return createdPoolMembers, t.statusUpdatePoolMember(ctx, poolId, poolName, createdPoolMembers)
}

// ComparePoolMembers compares two pool members.
// mustBeEqual is true if the two pool members must be equal, otherwise, just check if the pool members exist in the other pool members.
func (t *defaultModelDeployTask) buildPatchGlobalPoolMemberRequest(ctx context.Context, lbId, poolId string, currentPoolMembers *entityv2.ListGlobalPoolMembers, poolMembersSpec []v1alpha1.GlobalPoolMember) (global.IPatchGlobalPoolMembersRequest, []string) {
	searchPoolMemberByName := func(name string) *entityv2.GlobalPoolMember {
		for _, p := range currentPoolMembers.Items {
			if p.Name == name {
				return p
			}
		}
		return nil
	}

	searchCreatedPoolMemberById := func(id string) *v1alpha1.CreatedGlobalPoolMember {
		for _, createdPool := range t.lbConfig.Status.CreatedPools {
			if createdPool.Id != poolId {
				continue
			}
			for _, createdPoolMember := range createdPool.CreatedPoolMembers {
				if createdPoolMember.Id == id {
					return &createdPoolMember
				}
			}
			break
		}
		return &v1alpha1.CreatedGlobalPoolMember{CreatedMembers: []v1alpha1.GlobalMember{}}
	}

	bulkRequests := make([]global.IBulkActionRequest, 0)
	allActionMessages := make([]string, 0)

	for _, poolMemberSpec := range poolMembersSpec {
		// add create action
		if currentPoolMember := searchPoolMemberByName(poolMemberSpec.Name); currentPoolMember == nil {
			createRequest, messages := t.buildCreatePoolMemberRequest(ctx, lbId, poolId, &poolMemberSpec)

			allActionMessages = append(allActionMessages, messages...)
			bulkRequests = append(bulkRequests, global.NewPatchGlobalPoolCreateBulkActionRequest(createRequest))
		} else { // add update action
			createdPoolMember := searchCreatedPoolMemberById(currentPoolMember.ID)

			if updateRequest, messages := t.buildUpdateGlobalPoolMemberRequest(ctx, lbId, poolId, currentPoolMember, &poolMemberSpec, createdPoolMember); updateRequest != nil {

				allActionMessages = append(allActionMessages, messages...)
				bulkRequests = append(bulkRequests, global.NewPatchGlobalPoolUpdateBulkActionRequest(poolId, updateRequest))
			}
		}
	}

	if len(bulkRequests) == 0 {
		return nil, nil
	}

	return global.NewPatchGlobalPoolMembersRequest(lbId, poolId).WithBulkAction(bulkRequests...), allActionMessages
}

func (t *defaultModelDeployTask) buildCreatePoolMemberRequest(_ context.Context, lbId, poolId string, poolMemberSpec *v1alpha1.GlobalPoolMember) (global.ICreateGlobalPoolMemberRequest, []string) {

	memberStrings := make([]string, len(poolMemberSpec.Members))
	for i, member := range poolMemberSpec.Members {
		memberStrings[i] = fmt.Sprintf("%s:%d", member.Address, member.Port)
	}

	globalMembersRequest := make([]global.IGlobalMemberRequest, 0, len(poolMemberSpec.Members))
	for _, memberSpec := range poolMemberSpec.Members {
		// Use default values for optional fields
		monitorPort := memberSpec.Port // default to member port
		if memberSpec.MonitorPort != nil {
			monitorPort = *memberSpec.MonitorPort
		}
		weight := 1 // default weight
		if memberSpec.Weight != nil {
			weight = *memberSpec.Weight
		}

		memberRequest := global.NewGlobalMemberRequest(
			memberSpec.Name,
			memberSpec.Address,
			memberSpec.SubnetID,
			memberSpec.Port,
			monitorPort,
			weight,
			memberSpec.BackupRole,
		)

		globalMembersRequest = append(globalMembersRequest, memberRequest)
	}

	// Use default traffic dial if not specified
	trafficDial := 100 // default to 100%
	if poolMemberSpec.TrafficDial != nil {
		trafficDial = *poolMemberSpec.TrafficDial
	}

	poolMemberRequest := global.NewGlobalPoolMemberRequest(
		poolMemberSpec.Name,
		poolMemberSpec.Region,
		poolMemberSpec.VpcId,
		trafficDial,
		poolMemberSpec.Type,
	)
	poolMemberRequest.WithMembers(globalMembersRequest...)

	return poolMemberRequest, []string{fmt.Sprintf("add %s [%s]", poolMemberSpec.Name, strings.Join(memberStrings, ", "))}
}

func (t *defaultModelDeployTask) buildUpdateGlobalPoolMemberRequest(ctx context.Context, lbId, poolId string, currentPoolMember *entityv2.GlobalPoolMember, poolMemberSpec *v1alpha1.GlobalPoolMember, createdPoolMember *v1alpha1.CreatedGlobalPoolMember) (global.IUpdateGlobalPoolMemberRequest, []string) {
	result := global.NewUpdateGlobalPoolMemberRequest(lbId, poolId, currentPoolMember.ID, currentPoolMember.TrafficDial)
	message := make([]string, 0)
	isNeedUpdate := false

	if poolMemberSpec.TrafficDial != nil && currentPoolMember.TrafficDial != *poolMemberSpec.TrafficDial {
		message = append(message, fmt.Sprintf("traffic dial (%d -> %d)", currentPoolMember.TrafficDial, poolMemberSpec.TrafficDial))
		isNeedUpdate = true
	}

	if updatePoolMembersOption := t.buildGlobalMemberRequest(ctx, currentPoolMember.Members, poolMemberSpec.Members, createdPoolMember.CreatedMembers); updatePoolMembersOption != nil {
		result.WithMembers(updatePoolMembersOption...)
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil
	}

	return result, []string{fmt.Sprintf("update %s: [%s]", poolMemberSpec.Name, strings.Join(message, ", "))}
}

// compare current, expected, created
func (t *defaultModelDeployTask) buildGlobalMemberRequest(ctx context.Context, currentMembers *entityv2.ListGlobalMembers, membersSpec []v1alpha1.GlobalMember, createdMembers []v1alpha1.GlobalMember) []global.IGlobalMemberRequest {

	updateMembers := t.mergePoolMembers(ctx,
		createdMembers,
		convertMemberList(currentMembers),
		membersSpec)

	if comparePoolMembers(updateMembers, convertMemberList(currentMembers)) {
		return nil
	}

	membersRequest := make([]global.IGlobalMemberRequest, 0)
	for _, member := range updateMembers {
		// Use default values for optional fields
		monitorPort := member.Port // default to member port
		if member.MonitorPort != nil {
			monitorPort = *member.MonitorPort
		}
		weight := 1 // default weight
		if member.Weight != nil {
			weight = *member.Weight
		}

		memberRequest := global.NewGlobalMemberRequest(
			member.Name,
			member.Address,
			member.SubnetID,
			member.Port,
			monitorPort,
			weight,
			member.BackupRole,
		)
		membersRequest = append(membersRequest, memberRequest)
	}
	return membersRequest
}

// MergePoolMembers merges the pool members
// - keep current member if it is in spec or not created by us
func (t *defaultModelDeployTask) mergePoolMembers(_ context.Context, createdMembers, currentMembers, poolMemberSpec []v1alpha1.GlobalMember) []v1alpha1.GlobalMember {
	mergedPoolMembers := make([]v1alpha1.GlobalMember, 0)

	// keep current member if it is in spec or not created by us
	for _, member := range currentMembers {
		if checkIfPoolMemberExist(poolMemberSpec, &member) || !checkIfPoolMemberExist(createdMembers, &member) {
			mergedPoolMembers = append(mergedPoolMembers, member)
		}
	}

	// add new members from spec
	for _, member := range poolMemberSpec {
		if !checkIfPoolMemberExist(mergedPoolMembers, &member) {
			mergedPoolMembers = append(mergedPoolMembers, member)
		}
	}

	return mergedPoolMembers
}

func comparePoolMembers(poolMembers []v1alpha1.GlobalMember, current []v1alpha1.GlobalMember) bool {
	if len(poolMembers) != len(current) {
		return false
	}

	for _, member := range poolMembers {
		if !checkIfPoolMemberExist(current, &member) {
			return false
		}
	}

	return true
}

// checkIfPoolMemberExist checks if the pool member exists in the pool members.
func checkIfPoolMemberExist(list []v1alpha1.GlobalMember, member *v1alpha1.GlobalMember) bool {
	for _, r := range list {
		if r.Address == member.Address &&
			r.Port == member.Port &&
			r.Weight == member.Weight &&
			r.BackupRole == member.BackupRole &&
			r.SubnetID == member.SubnetID &&
			r.MonitorPort == member.MonitorPort {
			return true
		}
	}
	return false
}

func convertMemberList(members *entityv2.ListGlobalMembers) []v1alpha1.GlobalMember {
	result := make([]v1alpha1.GlobalMember, 0)
	if members == nil || len(members.Items) == 0 {
		return result
	}
	for _, member := range members.Items {
		result = append(result, *convertMember(member))
	}
	return result
}

func convertMember(member *entityv2.GlobalPoolMemberDetail) *v1alpha1.GlobalMember {
	return &v1alpha1.GlobalMember{
		Name:        member.Name,
		Address:     member.Address,
		Port:        member.Port,
		BackupRole:  member.BackupRole,
		Weight:      &member.Weight,
		MonitorPort: &member.MonitorPort,
	}
}
