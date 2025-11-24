package nsg_uc

import (
	"context"
	"strings"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	k8sRepo      repository.K8sRepository
	vngcloudRepo repository.VngCloudRepository
}

func NewNodeSecurityGroupUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
) usecase.NodeSecurityGroupUseCase {
	return &nsgUseCase{
		cfg:          cfg,
		k8sRepo:      k8sRepo,
		vngcloudRepo: vngcloudRepo,
	}
}

func (uc *nsgUseCase) InitNodeSecurityGroupUseCase(ctx context.Context) error {
	return nil
}

func (uc *nsgUseCase) EnsureNodeSecurityGroupUseCase(ctx context.Context, req ctrl.Request) error {
	nsgObject, err := uc.k8sRepo.GetNodeSecurityGroup(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Perform the actual reconciliation
	err = uc.ensure(ctx, nsgObject)

	// Update reconciliation tracking fields
	now := metav1.Now()
	message := "Successfully reconciled"
	if err != nil {
		message = err.Error()
	}

	// !IMPORTANT!: The tests will fail without this
	statusErr := uc.k8sRepo.PatchMutateStatusNodeSecurityGroup(ctx, nsgObject, func(ctx context.Context, obj *v1alpha1.NodeSecurityGroup) {
		obj.Status.ObservedGeneration = obj.Generation
		obj.Status.LastReconcileTime = &now
		obj.Status.LastReconcileMessage = message
	})
	if statusErr != nil {
		logger := contexts.NewContext(ctx).Log()
		logger.Warnf("Failed to update reconciliation tracking fields: %v", statusErr)
	}

	return err
}

func (uc *nsgUseCase) ensure(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) error {

	managedSecgroupStatus, err := uc.ensureManagedSecurityGroup(ctx, nsgObject)
	if err != nil {
		return err
	}

	nodeInfos, err := uc.ensureSelectedNodes(ctx, nsgObject)
	if err != nil {
		return err
	}

	ssgs, err := uc.ensureServerSecurityGroups(ctx, nsgObject, nodeInfos, *managedSecgroupStatus)
	if err != nil {
		return err
	}
	if err := uc.statusServerSecurityGroupStatus(ctx, nsgObject, ssgs); err != nil {
		return err
	}
	return nil
}

// ensureSelectedNodes ensures that the selected nodes are updated in the status.selectedNodes field
func (uc *nsgUseCase) ensureSelectedNodes(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup) ([]v1alpha1.NodeInfo, error) {
	// collect new server IDs
	listNodes, err := uc.listNodeBySelector(ctx, nsgObject.Spec.SelectNodeLabels)
	if err != nil {
		return nil, err
	}

	nodeInfos := make([]v1alpha1.NodeInfo, 0)
	if listNodes != nil && len(listNodes.Items) > 0 {
		nodeInfos = make([]v1alpha1.NodeInfo, len(listNodes.Items))
		for i, node := range listNodes.Items {
			providerID := utils.GetProviderIdFromNode(&node)
			nodeInfos[i] = v1alpha1.NodeInfo{
				Name:     node.Name,
				ServerId: providerID,
			}
		}
	}

	// patch status.selectedNodes
	if err := uc.statusSetSelectedNodes(ctx, nsgObject, nodeInfos); err != nil {
		return nil, err
	}
	return nodeInfos, nil
}

func (uc *nsgUseCase) ensureServerSecurityGroups(ctx context.Context, nsgObject *v1alpha1.NodeSecurityGroup, nodeInfos []v1alpha1.NodeInfo, managedSecgroupStatus v1alpha1.ManagedSecurityGroupStatus) ([]v1alpha1.ServerSecurityGroupStatus, error) {
	// collect old server IDs
	oldServerSelector := make([]string, 0)
	for _, nodeInfo := range nsgObject.Status.SelectedNodes {
		oldServerSelector = append(oldServerSelector, nodeInfo.ServerId)
	}

	newServerSelector := make([]string, len(nodeInfos))
	for i, nodeInfo := range nodeInfos {
		newServerSelector[i] = nodeInfo.ServerId
	}

	// attach new security groups in spec
	needAttachSecgroupIds := make([]string, 0)
	needAttachSecgroupIds = append(needAttachSecgroupIds, nsgObject.Spec.AttachSecurityGroups...)

	if managedSecgroupStatus.Id != nil && *managedSecgroupStatus.Id != "" {
		needAttachSecgroupIds = append(needAttachSecgroupIds, *managedSecgroupStatus.Id)
	}

	// resolve changes
	removeServerIDs, notChangeServerIDs, addServerIDs := resolveStringArrayChange(ctx,
		oldServerSelector,
		newServerSelector,
	)

	logger := contexts.NewContext(ctx).Log()

	result := make([]v1alpha1.ServerSecurityGroupStatus, 0)

	errorsList := make([]error, 0)
	// detach security groups from removed servers
	for _, serverId := range removeServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverId, []string{})
		if err != nil && !domain.IsServerNotFound(err) {
			errorsList = append(errorsList, err)
		}
	}

	// update security groups for not changed servers
	for _, serverId := range notChangeServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverId, needAttachSecgroupIds)
		if err != nil {
			errorsList = append(errorsList, err)
		}
		_err := uc.statusUpdateNodeSecurityGroup(ctx, nsgObject, serverId, err, needAttachSecgroupIds)
		if _err != nil {
			logger.Warn("Fail to update status for server: ", serverId, " error: ", _err)
		}
		result = append(result, v1alpha1.ServerSecurityGroupStatus{
			ServerId:                 serverId,
			AttachedSecurityGroupIds: needAttachSecgroupIds,
			Error:                    errorToStringPtr(err),
		})
	}

	// attach security groups to added servers
	for _, serverId := range addServerIDs {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, serverId, needAttachSecgroupIds)
		if err != nil {
			errorsList = append(errorsList, err)
		}
		_err := uc.statusUpdateNodeSecurityGroup(ctx, nsgObject, serverId, err, needAttachSecgroupIds)
		if _err != nil {
			logger.Warn("Fail to update status for server: ", serverId, " error: ", _err)
		}
		result = append(result, v1alpha1.ServerSecurityGroupStatus{
			ServerId:                 serverId,
			AttachedSecurityGroupIds: needAttachSecgroupIds,
			Error:                    errorToStringPtr(err),
		})
	}

	// try delete managed security group if not in use and not attached to any server
	if nsgObject.Status.ManagedSecurityGroup.Id != nil && *nsgObject.Status.ManagedSecurityGroup.Id != "" {
		isDeleted, err := uc.deleteManagedSecurityGroupIfUnused(ctx, *nsgObject.Status.ManagedSecurityGroup.Id)
		if err != nil {
			uc.statusAddStatusManagedSecurityGroup(ctx, nsgObject, nsgObject.Status.ManagedSecurityGroup.Id, err)
			return nil, err
		}
		if isDeleted {
			// patch status
			if err := uc.statusAddStatusManagedSecurityGroup(ctx, nsgObject, nil, nil); err != nil {
				return nil, err
			}
		}
	}

	if len(errorsList) > 0 {
		errMessages := make([]string, 0)
		for _, err := range errorsList {
			errMessages = append(errMessages, err.Error())
		}
		return nil, errors.New("failed to update security groups for some nodes: " + strings.Join(errMessages, ", "))
	}

	return result, nil
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

func (uc *nsgUseCase) DeleteNodeSecurityGroupUseCase(ctx context.Context, req ctrl.Request) error {
	nsgObject, err := uc.k8sRepo.GetNodeSecurityGroup(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// detach security groups from all selected servers
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Detach security groups: %v", nsgObject.Status.ServerSecurityGroups)
	errorsList := make([]error, 0)
	for _, server_secgroup := range nsgObject.Status.ServerSecurityGroups {
		err := uc.ensureSecgroupForInstance(ctx, nsgObject, server_secgroup.ServerId, []string{})
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
		_, err := uc.deleteManagedSecurityGroupIfUnused(ctx, *nsgObject.Status.ManagedSecurityGroup.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

// deleteManagedSecurityGroupIfUnused deletes the managed security group if it is not attached to any server
// and returns isDeleted and error
func (uc *nsgUseCase) deleteManagedSecurityGroupIfUnused(ctx context.Context, secgroupID string) (bool, error) {
	logger := contexts.NewContext(ctx).Log()

	// check if secgroup is exists
	if _, err := uc.vngcloudRepo.GetSecurityGroup(ctx, secgroupID); err != nil {
		if domain.IsSecurityGroupNotFound(err) {
			logger.Infof("Security group %s not found, skip delete", secgroupID)
			return true, nil
		}
		return false, err
	}

	serverList, err := uc.vngcloudRepo.ListServerBySecgroupID(ctx, secgroupID)
	if err != nil {
		return false, err
	}

	length := 0
	if serverList == nil || len(serverList.Items) == 0 {
		length = 0
	} else {
		length = len(serverList.Items)
	}

	if length > 0 {
		serverIds := make([]string, 0)
		for _, server := range serverList.Items {
			serverIds = append(serverIds, server.Uuid)
		}
		logger.Infof("Security group %s is still attached to servers: %v. Skip delete.", secgroupID, serverIds)
		return false, nil
	}

	err = uc.vngcloudRepo.DeleteSecurityGroup(ctx, secgroupID)
	if err != nil {
		return false, err
	}
	return true, nil
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
		if server_secgroup.ServerId == instanceID {
			oldSecgroupIds = append(oldSecgroupIds, server_secgroup.AttachedSecurityGroupIds...)
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

// errorToStringPtr converts an error to a string pointer, returns nil if error is nil
func errorToStringPtr(err error) *string {
	if err == nil {
		return nil
	}
	errMsg := err.Error()
	return &errMsg
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
func resolveStringArrayChange(ctx context.Context, oldArr, newArr []string) (removeArr, notChangeArr, addArr []string) {
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

	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("- old array: %v, new array: %v", oldArr, newArr)
	logger.Debugf("- remove %v, not change %v, add %v", removeArr, notChangeArr, addArr)

	return removeArr, notChangeArr, addArr
}
