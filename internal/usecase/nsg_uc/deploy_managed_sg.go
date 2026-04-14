package nsg_uc

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// ensureManagedSecurityGroup ensures that the managed security group is created and updated according to the NSG spec
// returns the managed security group ID
func (uc *nsgUseCase) ensureManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) (*v1alpha1.ManagedSecurityGroupStatus, error) {
	// patch status at the end
	status, err := uc.doEnsureManagedSecurityGroup(ctx, nsgObject)
	patchErr := uc.statusAddStatusManagedSecurityGroup(ctx, nsgObject, status.Id, err)
	if err != nil {
		// operation error takes priority; log patch failure separately
		if patchErr != nil {
			logger := contexts.NewContext(ctx).Log()
			logger.Warnf("Failed to update managed secgroup status: %v", patchErr)
		}
		return status, err
	}
	return status, patchErr
}

func (uc *nsgUseCase) doEnsureManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) (*v1alpha1.ManagedSecurityGroupStatus, error) {
	status := &v1alpha1.ManagedSecurityGroupStatus{}

	if nsgObject.Spec.ManagedSecurityGroup == nil {
		return status, nil
	}

	logger := contexts.NewContext(ctx).Log()

	// find default secgroup
	secgroup, exists, err := uc.findSecgroupByName(ctx, nsgObject.Spec.ManagedSecurityGroup.Name)
	if err != nil {
		logger.Error("Fail to find default secgroup by name", err)
		return status, err
	}

	if !exists {
		// create new secgroup if not exists
		description := "Automatically created using VNGCLOUD LoadBalancer Controller"
		if nsgObject.Spec.ManagedSecurityGroup.Description != nil {
			description = *nsgObject.Spec.ManagedSecurityGroup.Description
		}
		created, err := uc.vngcloudRepo.CreateSecurityGroup(ctx, nsgObject.Spec.ManagedSecurityGroup.Name, description)
		if err != nil {
			logger.Error("Fail to create default secgroup", err)
			return status, err
		}
		secgroup, err = uc.vngcloudRepo.GetSecurityGroup(ctx, created.Id)
		if err != nil {
			logger.Error("Fail to get default secgroup", err)
			return status, err
		}
	}

	// add secgroup id to status
	status.Id = &secgroup.Id

	// update secgroup rules if needed

	// get all secgroup rules of default secgroup
	rules, err := uc.vngcloudRepo.ListSecurityGroupRules(ctx, secgroup.Id)
	if err != nil {
		logger.Error("Fail to list secgroup rules: ", err)
		return status, err
	}
	if rules == nil || rules.Items == nil {
		rules = &entityv2.ListSecgroupRules{Items: []*entityv2.SecgroupRule{}}
	}

	logger.Debug("Ensure secgroup rules: ")
	for _, rule := range rules.Items {
		logger.Debugf("   - current: %v", rule)
	}

	// ensure secgroup rules
	needDelete, needCreate, err := uc.compareSecgroupRule(ctx, rules.Items, nsgObject.Spec.ManagedSecurityGroup.Rules)
	if err != nil {
		logger.Error("Fail to compare secgroup rules", err)
		return status, err
	}

	for _, rule := range needDelete {
		logger.Debugf("   - delete : %v", rule)
	}
	for _, rule := range needCreate {
		logger.Debugf("   - create : %v", rule)
	}

	for _, rule := range needDelete {
		if err := uc.vngcloudRepo.DeleteSecurityGroupRule(ctx, secgroup.Id, rule.Id); err != nil {
			logger.Error("Fail to delete secgroup rule", err)
			return status, err
		}
	}

	for _, rule := range needCreate {
		_, err := uc.vngcloudRepo.CreateSecurityGroupRule(ctx, secgroup.Id,
			networkv2.NewCreateSecgroupRuleRequest(
				rule.Direction,
				rule.EtherType,
				rule.Protocol,
				int(rule.FromPort),
				int(rule.ToPort),
				rule.CIDR,
				secgroup.Id,
				rule.Description,
			))
		if err != nil {
			logger.Error("Fail to create secgroup rule: ", err)
			return status, err
		}
	}

	return status, nil
}
