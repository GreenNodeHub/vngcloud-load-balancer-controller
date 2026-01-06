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
func (uc *nsgUseCase) ensureManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) (finalStatus *v1alpha1.ManagedSecurityGroupStatus, finalErr error) {
	// path status at the end
	finalStatus = &v1alpha1.ManagedSecurityGroupStatus{}
	finalErr = nil
	defer func() {
		_err := uc.statusAddStatusManagedSecurityGroup(ctx, nsgObject, finalStatus.Id, finalErr)
		if finalErr != nil {
			finalErr = _err
			return
		}
	}()

	if nsgObject.Spec.ManagedSecurityGroup == nil {
		return
	}

	logger := contexts.NewContext(ctx).Log()
	// find default secgroup
	defaultSecgroup, isExists, err := uc.findSecgroupByName(ctx, nsgObject.Spec.ManagedSecurityGroup.Name)
	if err != nil {
		logger.Error("Fail to find default secgroup by name", err)
		finalErr = err
		return
	}
	if !isExists {
		// create new secgroup if not exists
		description := "Automatically created using VNGCLOUD LoadBalancer Controller"
		if nsgObject.Spec.ManagedSecurityGroup.Description != nil {
			description = *nsgObject.Spec.ManagedSecurityGroup.Description
		}
		_secG, err := uc.vngcloudRepo.CreateSecurityGroup(ctx, nsgObject.Spec.ManagedSecurityGroup.Name, description)
		if err != nil {
			logger.Error("Fail to create default secgroup", err)
			finalErr = err
			return
		}
		defaultSecgroup, err = uc.vngcloudRepo.GetSecurityGroup(ctx, _secG.Id)
		if err != nil {
			logger.Error("Fail to get default secgroup", err)
			finalErr = err
			return
		}
	}

	// add secgroup id to status
	finalStatus.Id = &defaultSecgroup.Id

	// update secgroup rules if needed

	// get all secgroup rules of default secgroup
	defaultSecgroupRules, err := uc.vngcloudRepo.ListSecurityGroupRules(ctx, defaultSecgroup.Id)
	if err != nil {
		logger.Error("Fail to list secgroup rules: ", err)
		finalErr = err
		return
	}

	if defaultSecgroupRules == nil || defaultSecgroupRules.Items == nil {
		defaultSecgroupRules = &entityv2.ListSecgroupRules{
			Items: []*entityv2.SecgroupRule{},
		}
	}

	logger.Debug("Ensure secgroup rules: ")
	for _, rule := range defaultSecgroupRules.Items {
		if rule.Direction == string(networkv2.SecgroupRuleDirectionEgress) {
			continue
		}
		logger.Debugf("   - current: %v", rule)
	}

	// ensure secgroup rules
	needDelete, needCreate, err := uc.compareSecgroupRule(ctx, defaultSecgroupRules.Items, nsgObject.Spec.ManagedSecurityGroup.Rules)
	if err != nil {
		logger.Error("Fail to compare secgroup rules", err)
		finalErr = err
		return
	}

	for _, rule := range needDelete {
		logger.Debugf("   - delete : %v", rule)
	}
	for _, rule := range needCreate {
		logger.Debugf("   - create : %v", rule)
	}

	for _, rule := range needDelete {
		err := uc.vngcloudRepo.DeleteSecurityGroupRule(ctx, defaultSecgroup.Id, rule.Id)
		if err != nil {
			logger.Error("Fail to delete secgroup rule", err)
			finalErr = err
			return
		}
	}

	for _, rule := range needCreate {
		_, err := uc.vngcloudRepo.CreateSecurityGroupRule(ctx, defaultSecgroup.Id,
			networkv2.NewCreateSecgroupRuleRequest(
				rule.Direction,
				rule.EtherType,
				rule.Protocol,
				int(rule.FromPort),
				int(rule.ToPort),
				rule.CIDR,
				defaultSecgroup.Id,
				rule.Description,
			))
		if err != nil {
			logger.Error("Fail to create secgroup rule: ", err)
			finalErr = err
			return
		}
	}

	return
}
