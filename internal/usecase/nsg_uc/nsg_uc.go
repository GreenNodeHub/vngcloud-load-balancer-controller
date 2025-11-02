package nsg_uc

import (
	"context"
	"strings"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type nsgUseCase struct {
	cfg          *config.Config
	k8sRepo      repository.IK8sRepository
	vngcloudRepo repository.IVngCloudRepository
}

func NewNSGUseCase(
	cfg *config.Config,
	k8sRepo repository.IK8sRepository,
	vngcloudRepo repository.IVngCloudRepository,
) usecase.NodeSecurityGroupUseCase {
	return &nsgUseCase{
		cfg:          cfg,
		k8sRepo:      k8sRepo,
		vngcloudRepo: vngcloudRepo,
	}
}

func (uc *nsgUseCase) Init(ctx context.Context) error {
	return nil
}

func (uc *nsgUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
	nsgObject, err := uc.k8sRepo.GetNodeSecurityGroup(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	managedSecgroupID, err := uc.ensureMagagedSecurityGroup(ctx, nsgObject)
	uc.ensureStatusManagedSecurityGroup(ctx, nsgObject, managedSecgroupID, err)
	if err != nil {
		return err
	}

	// attach new security groups in spec
	needAttachSecgroupIds := make([]string, 0)
	needAttachSecgroupIds = append(needAttachSecgroupIds, nsgObject.Spec.AttachSecurityGroups...)

	if managedSecgroupID != "" {
		needAttachSecgroupIds = append(needAttachSecgroupIds, managedSecgroupID)
	}

	// collect old server IDs
	oldServerSelector := make([]string, 0)
	for _, nodeInfo := range nsgObject.Status.SelectedNodes {
		oldServerSelector = append(oldServerSelector, nodeInfo.ServerID)
	}

	// collect new server IDs
	listNodes, err := uc.listNodeBySelector(ctx, nsgObject.Spec.SelectNodeLabels)
	if err != nil {
		return err
	}
	newServerSelector := utils.GetListProviderIdFromNodeList(listNodes)

	// patch status.selectedNodes
	err = uc.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject, func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
		nodeInfos := make([]v1alpha1.NodeInfo, len(newServerSelector))
		for i, serverID := range newServerSelector {
			nodeInfos[i] = v1alpha1.NodeInfo{
				Name:     serverID, // TODO: get node name from serverID
				ServerID: serverID,
			}
		}
		obj.Status.SelectedNodes = nodeInfos
	})
	if err != nil {
		return err
	}

	// resolve changes
	removeServerIDs, notChangeServerIDs, addServerIDs := resolveStringArrayChange(
		oldServerSelector,
		newServerSelector,
	)

	logger := contexts.NewContext(ctx).Log()

	errorsList := make([]error, 0)
	// detach security groups from removed servers
	for _, serverID := range removeServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverID, []string{})
		if err != nil {
			errorsList = append(errorsList, err)
		}
		_err := uc.ensureStatusNodeSecurityGroup(ctx, nsgObject, serverID, err, []string{})
		if _err != nil {
			logger.Warn("Fail to update status for server: ", serverID, " error: ", _err)
		}
	}

	// update security groups for not changed servers
	for _, serverID := range notChangeServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverID, needAttachSecgroupIds)
		if err != nil {
			errorsList = append(errorsList, err)
		}
		_err := uc.ensureStatusNodeSecurityGroup(ctx, nsgObject, serverID, err, needAttachSecgroupIds)
		if _err != nil {
			logger.Warn("Fail to update status for server: ", serverID, " error: ", _err)
		}
	}

	// attach security groups to added servers
	for _, serverID := range addServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverID, needAttachSecgroupIds)
		if err != nil {
			errorsList = append(errorsList, err)
		}
		_err := uc.ensureStatusNodeSecurityGroup(ctx, nsgObject, serverID, err, needAttachSecgroupIds)
		if _err != nil {
			logger.Warn("Fail to update status for server: ", serverID, " error: ", _err)
		}
	}

	// try delete managed security group if not in use and not attached to any server
	if managedSecgroupID == "" {
		if nsgObject.Status.ManagedSecurityGroup.Id != nil && *nsgObject.Status.ManagedSecurityGroup.Id != "" {
			err := uc.deleteManagedSecurityGroupIfUnused(ctx, *nsgObject.Status.ManagedSecurityGroup.Id)
			if err != nil {
				uc.ensureStatusManagedSecurityGroup(ctx, nsgObject, *nsgObject.Status.ManagedSecurityGroup.Id, err)
				return err
			}
			// patch status
			uc.ensureStatusManagedSecurityGroup(ctx, nsgObject, "", nil)
		}
	}

	if len(errorsList) > 0 {
		errMessages := make([]string, 0)
		for _, err := range errorsList {
			errMessages = append(errMessages, err.Error())
		}
		return errors.New("failed to update security groups for some nodes: " + strings.Join(errMessages, ", "))
	}

	return nil
}

// ensureMagagedSecurityGroup ensures that the managed security group is created and updated according to the NSG spec
// returns the managed security group ID
func (uc *nsgUseCase) ensureMagagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) (string, error) {
	if nsgObject.Spec.ManagedSecurityGroup == nil {
		return "", nil
	}

	logger := contexts.NewContext(ctx).Log()
	// find default secgroup
	defaultSecgroup, isExists, err := uc.findSecgroupByName(ctx, nsgObject.Spec.ManagedSecurityGroup.Name)
	if err != nil {
		logger.Error("Fail to find default secgroup", err)
		return "", err
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
			return "", err
		}
		defaultSecgroup, err = uc.vngcloudRepo.GetSecurityGroup(ctx, _secG.Id)
		if err != nil {
			logger.Error("Fail to get default secgroup", err)
			return "", err
		}
	}

	// update secgroup rules if needed

	// get all secgroup rules of default secgroup
	defaultSecgroupRules, err := uc.vngcloudRepo.ListSecurityGroupRules(ctx, defaultSecgroup.Id)
	if err != nil {
		logger.Error("Fail to list secgroup rules: ", err)
		return "", err
	}

	if defaultSecgroupRules == nil || defaultSecgroupRules.Items == nil {
		defaultSecgroupRules = &entityv2.ListSecgroupRules{
			Items: []*entityv2.SecgroupRule{},
		}
	}

	// TODO: should add engress rules too
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
		return "", err
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
			return "", err
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
			return "", err
		}
	}
	return defaultSecgroup.Id, nil
}

// findSecgroupByName finds a security group by name
func (uc *nsgUseCase) findSecgroupByName(ctx context.Context, name string) (*entityv2.Secgroup, bool, error) {
	secgroups, err := uc.vngcloudRepo.ListSecurityGroups(ctx)
	if err != nil {
		return nil, false, err
	}

	for _, secgroup := range secgroups.Items {
		if secgroup.Name == name {
			return secgroup, true, nil
		}
	}

	return nil, false, nil
}

// compareSecgroupRule compares current security group rules with new rules,
// returns the rules that need to be deleted and created
func (uc *nsgUseCase) compareSecgroupRule(_ context.Context, currentRules []*entityv2.SecgroupRule, newRules []v1alpha1.NodeSecurityGroupRule) ([]*entityv2.SecgroupRule, []v1alpha1.NodeSecurityGroupRule, error) {
	needDelete := make([]*entityv2.SecgroupRule, 0)
	needCreate := make([]v1alpha1.NodeSecurityGroupRule, 0)

	// mark all rule not in use
	ruleInUse := make(map[string]bool)
	for _, rule := range currentRules {
		ruleInUse[rule.Id] = false
	}

	// check if the rule is in new
	for _, newRule := range newRules {
		found := false
		for _, currentRule := range currentRules {
			if strings.EqualFold(string(newRule.Direction), currentRule.Direction) &&
				strings.EqualFold(string(newRule.Protocol), currentRule.Protocol) &&
				// newRule.Description == currentRule.Description &&
				strings.EqualFold(string(newRule.EtherType), currentRule.EtherType) &&
				int(newRule.ToPort) == currentRule.PortRangeMax &&
				int(newRule.FromPort) == currentRule.PortRangeMin &&
				newRule.CIDR == currentRule.RemoteIPPrefix {
				ruleInUse[currentRule.Id] = true
				found = true
				break
			}
		}

		if !found {
			needCreate = append(needCreate, newRule)
		}
	}

	// check if the rule is not in use
	for _, rule := range currentRules {
		if !ruleInUse[rule.Id] {
			needDelete = append(needDelete, rule)
		}
	}

	return needDelete, needCreate, nil
}

func (uc *nsgUseCase) listNodeBySelector(ctx context.Context, selector map[string]string) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes, client.MatchingLabels(selector))
	if err != nil {
		logger := contexts.NewContext(ctx).Log()
		logger.Error("Fail to list nodes: ", err)
		return nil, err
	}
	return nodes, nil
}

func (uc *nsgUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	nsgObject, err := uc.k8sRepo.GetNodeSecurityGroup(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	errorsList := make([]error, 0)
	for _, server_secgroup := range nsgObject.Status.ServerSecurityGroups {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, server_secgroup.ServerID, []string{})
		if err != nil {
			errorsList = append(errorsList, err)
		}
	}
	if len(errorsList) > 0 {
		errMessages := make([]string, 0)
		for _, err := range errorsList {
			errMessages = append(errMessages, err.Error())
		}
		return errors.New("failed to detach security groups from some nodes: " + strings.Join(errMessages, ", "))
	}

	// try delete managed security group if not used by any server
	if nsgObject.Status.ManagedSecurityGroup.Id != nil && *nsgObject.Status.ManagedSecurityGroup.Id != "" {
		err := uc.deleteManagedSecurityGroupIfUnused(ctx, *nsgObject.Status.ManagedSecurityGroup.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

// deleteManagedSecurityGroupIfUnused deletes the managed security group if it is not attached to any server
func (uc *nsgUseCase) deleteManagedSecurityGroupIfUnused(ctx context.Context, secgroupID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Delete managed security group %s if unused", secgroupID)

	// check if secgroup is exists
	if _, err := uc.vngcloudRepo.GetSecurityGroup(ctx, secgroupID); err != nil {
		if domain.IsSecurityGroupNotFound(err) {
			logger.Infof("Security group %s not found, skip delete", secgroupID)
			return nil
		}
		return err
	}

	serverList, err := uc.vngcloudRepo.ListServerBySecgroupID(ctx, secgroupID)
	if err != nil {
		return err
	}

	length := 0
	if serverList == nil || len(serverList.Items) == 0 {
		length = 0
	} else {
		length = len(serverList.Items)
	}

	if length == 0 {
		err := uc.vngcloudRepo.DeleteSecurityGroup(ctx, secgroupID)
		if err != nil {
			return err
		}
	}
	return nil
}

// ensure secgroup for instance, get the current secgroups of instance,
// try remove old secgroups and try add new secgroups
func (m *nsgUseCase) ensureSecgroupForInstance(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, instanceID string, newSecgroupIds []string) error {
	logger := contexts.NewContext(ctx).Log()
	// get security groups of instance
	instance, err := m.vngcloudRepo.GetServerByID(ctx, instanceID)
	if err != nil {
		logger.Error("Fail to get instance: ", err)
		return err
	}

	currentSecgroupIds := make([]string, 0)
	for _, secgroup := range instance.SecGroups {
		currentSecgroupIds = append(currentSecgroupIds, secgroup.Uuid)
	}

	oldSecgroupIds := make([]string, 0)
	for _, server_secgroup := range nsgObject.Status.ServerSecurityGroups {
		if server_secgroup.ServerID == instanceID {
			oldSecgroupIds = append(oldSecgroupIds, server_secgroup.AttachedSecurityGroupIDs...)
			break
		}
	}

	newSecgroupIds, isNeedUpdate := mergeStringArray(ctx, currentSecgroupIds, oldSecgroupIds, newSecgroupIds)
	if !isNeedUpdate {
		logger.Infof("No need to update security groups for instance: %v", instanceID)
		return nil
	}

	// update security groups of instance
	logger.Infof("Update security groups of instance %s: %v -> %v", instanceID, currentSecgroupIds, newSecgroupIds)
	_, err = m.vngcloudRepo.UpdateSecGroupsOfServer(ctx, instanceID, newSecgroupIds)
	if err != nil {
		logger.Error("Fail to update security groups of instance: ", err)
		return err
	}

	// wait until the server is active
	err = m.vngcloudRepo.WaitForServerActive(ctx, instanceID)
	if err != nil {
		logger.Error("Fail to wait for server active: ", err)
		return err
	}

	return nil
}

// update the status.serverSecurityGroups of nsgObject for a specific server
func (m *nsgUseCase) ensureStatusNodeSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, serverId string, err error, attachedSecgroupIds []string) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
			// create a copy of current server security groups
			newServerSecurityGroups := make([]v1alpha1.ServerSecurityGroupStatus, len(obj.Status.ServerSecurityGroups))
			copy(newServerSecurityGroups, obj.Status.ServerSecurityGroups)

			// find and update or create the status of this server to status.serverSecurityGroups
			found := false
			for i, serverSecgroup := range newServerSecurityGroups {
				if serverSecgroup.ServerID == serverId {
					found = true
					newServerSecurityGroups[i].AttachedSecurityGroupIDs = attachedSecgroupIds
					if err != nil {
						errMsg := err.Error()
						newServerSecurityGroups[i].Error = &errMsg
					} else {
						newServerSecurityGroups[i].Error = nil
					}
					break
				}
			}
			if !found {
				newStatus := v1alpha1.ServerSecurityGroupStatus{
					ServerID:                 serverId,
					AttachedSecurityGroupIDs: attachedSecgroupIds,
				}
				if err != nil {
					errMsg := err.Error()
					newStatus.Error = &errMsg
				}
				newServerSecurityGroups = append(newServerSecurityGroups, newStatus)
			}
			obj.Status.ServerSecurityGroups = newServerSecurityGroups
		})
}

func (m *nsgUseCase) ensureStatusManagedSecurityGroup(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, secgroupID string, err error) error {
	return m.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject,
		func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
			obj.Status.ManagedSecurityGroup.Id = &secgroupID
			if secgroupID == "" {
				obj.Status.ManagedSecurityGroup.Id = nil
			}
			if err != nil {
				errMsg := err.Error()
				obj.Status.ManagedSecurityGroup.Error = &errMsg
			} else {
				obj.Status.ManagedSecurityGroup.Error = nil
			}
		})
}

// mergeStringArray merges current string array by removing and adding elements,
// returns the merged array and a boolean indicating if there was any change
func mergeStringArray(ctx context.Context, current, remove, add []string) ([]string, bool) {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("  - current: %v", current)
	logger.Debugf("  - remove:  %v", remove)
	logger.Debugf("  - add:     %v", add)

	mapCurrent := make(map[string]bool)
	for _, c := range current {
		mapCurrent[c] = true
	}
	for _, r := range remove {
		delete(mapCurrent, r)
	}
	for _, a := range add {
		mapCurrent[a] = true
	}
	ret := make([]string, 0)
	for k := range mapCurrent {
		ret = append(ret, k)
	}
	if len(ret) != len(current) {
		return ret, true
	}
	for _, c := range current {
		if !mapCurrent[c] {
			return ret, true
		}
	}
	return ret, false
}

// this function is used to reolve the update the change of array of strings
// eg: old [1,2,3], new [2,3,4]
// => remove [1], notchange [2,3], add [4]
func resolveStringArrayChange(oldArr, newArr []string) (removeArr, notChangeArr, addArr []string) {
	removeMap := make(map[string]bool)
	newMap := make(map[string]bool)

	for _, oldItem := range oldArr {
		removeMap[oldItem] = true
	}

	for _, newItem := range newArr {
		newMap[newItem] = true
		if removeMap[newItem] {
			// item exists in both old and new array, so it's not changed
			notChangeArr = append(notChangeArr, newItem)
			delete(removeMap, newItem)
		} else {
			// item is new, so it's added
			addArr = append(addArr, newItem)
		}
	}

	// remaining items in removeMap are those that are in old array but not in new array
	for item := range removeMap {
		removeArr = append(removeArr, item)
	}

	return removeArr, notChangeArr, addArr
}
