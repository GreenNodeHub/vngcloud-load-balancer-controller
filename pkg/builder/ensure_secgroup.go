package builder

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/anngdinh/operator-helper/k8s"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	corev1 "k8s.io/api/core/v1"
)

/*
*** if user not config anything, we will use default value
	for cilium native routing
		- target type instance:
			- port NodePort
			- port PodPort (to route to the pod) -> need watching endpoint even it's instance type
		- target type ip:
			- port PodPort (of course)
	for other CNI, we will use default value
		- target type instance:
			- port NodePort
		- target type ip:
			- port PodPort

*** if user pass annotation secgroup, we will:
	- find our default secgroup and check if it is in use, otherwise, delete it
	- ensure all server using the secgroups, which are in the annotation

??? which server is apply the secgroup? all or just the sercer in target node?
	- if mode is instance, we will apply the secgroup to the server in target node
	- if mode is ip, we will apply the secgroup to all servers

Note:
	- if use UDP protocol, must open ICMP protocol too.
*/

func (r *vngcloudLBBuilder) EnsureSecurityGroups(newBuilder ModelBuilder, oldBuilder OldModelBuilder) error {
	// find default secgroup
	defaultSecgroup, isExists, err := r.findSecgroupByName(r.GetLoadBalancerDefaultName())
	if err != nil {
		r.logger.Error("Fail to find default secgroup", err)
		return err
	}

	// if default secgroup but newBuilder not use it, should delete it
	if isExists && !newBuilder.IsCreateDefaultSecgroup() &&
		!slices.Contains(newBuilder.GetSecurityGroupIDs(), defaultSecgroup.Id) {

		r.logger.Infof("Default secgroup %s should be deleted", defaultSecgroup.Id)

		// remove secgroups from all server in target node
		err = r.ensureDeleteAddNodesSG([]string{defaultSecgroup.Id}, []string{}, r.knownNodes)
		if err != nil {
			return err
		}

		// if default secgroup is not in use, delete it
		isInUse, err := r.getServerUseSecgroup(defaultSecgroup.Id)
		if err != nil {
			return err
		}
		if !isInUse {
			err = r.provider.DeleteSecurityGroup(r.context, defaultSecgroup.Id)
			if err != nil {
				r.logger.Error("Fail to delete default secgroup", err)
				return err
			}
		} else {
			r.logger.Warnf("Default secgroup %s is still in use, which should not be", defaultSecgroup.Id)
		}
	}

	// get selector node
	var nodes []*corev1.Node
	if newBuilder.GetTargetType() == TargetTypeIP {
		nodes = k8s.FilterNodeWithLabel(r.knownNodes, map[string]string{})
	} else {
		nodes = k8s.FilterNodeWithLabel(r.knownNodes, newBuilder.GetTargetNodeLabels())
	}

	// ensure all server using the secgroups, which are in the annotation
	if !newBuilder.IsCreateDefaultSecgroup() {
		r.ensureDeleteAddNodesSG(oldBuilder.GetOldSecGroups(), newBuilder.GetSecurityGroupIDs(), nodes)
		return nil
	}

	// if default secgroup not exists, create it
	if !isExists {
		_secG, err := r.provider.CreateSecurityGroup(context.Background(), r.GetLoadBalancerDefaultName(),
			"Automatically created using VNGCLOUD LoadBalancer Controller")
		if err != nil {
			r.logger.Error("Fail to create default secgroup", err)
			return err
		}
		defaultSecgroup, err = r.provider.GetSecurityGroup(context.Background(), _secG.Id)
		if err != nil {
			r.logger.Error("Fail to get default secgroup", err)
			return err
		}
	}

	// get all secgroup rules of default secgroup
	defaultSecgroupRules, err := r.provider.ListSecurityGroupRules(context.Background(), defaultSecgroup.Id)
	if err != nil {
		r.logger.Error("Fail to list secgroup rules", err)
		return err
	}

	if defaultSecgroupRules == nil || defaultSecgroupRules.Items == nil {
		defaultSecgroupRules = &entityv2.ListSecgroupRules{
			Items: []*entityv2.SecgroupRule{},
		}
	}

	r.logger.Debug("Ensure secgroup rules: ")
	for _, rule := range defaultSecgroupRules.Items {
		if rule.Direction == string(networkv2.SecgroupRuleDirectionEgress) {
			continue
		}
		r.logger.Debugf("   - current: %v", rule)
	}

	// ensure secgroup rules
	newBuilder.EnsureSecgroupPING_UDP()
	needDelete, needCreate, err := r.compareSecgroupRule(defaultSecgroupRules.Items, newBuilder.GetListDefaultSecgroupRules())
	if err != nil {
		r.logger.Error("Fail to compare secgroup rules", err)
		return err
	}

	for _, rule := range needDelete {
		r.logger.Debugf("   - delete : %v", rule)
	}
	for _, rule := range needCreate {
		r.logger.Debugf("   - create : %v", rule)
	}

	for _, rule := range needDelete {
		err := r.provider.DeleteSecurityGroupRule(context.Background(), defaultSecgroup.Id, rule.Id)
		if err != nil {
			r.logger.Error("Fail to delete secgroup rule", err)
			return err
		}
	}

	for _, rule := range needCreate {
		_, err := r.provider.CreateSecurityGroupRule(context.Background(), defaultSecgroup.Id,
			rule.GetICreateSecgroupRuleRequest(defaultSecgroup.Id))
		if err != nil {
			r.logger.Error("Fail to create secgroup rule", err)
			return err
		}
	}

	// ensure secgroup in server
	err = r.ensureDeleteAddNodesSG(oldBuilder.GetOldSecGroups(), []string{defaultSecgroup.Id}, nodes)
	if err != nil {
		r.logger.Error("Fail to ensure server have secgroup", err)
		return err
	}

	return nil
}

func (r *vngcloudLBBuilder) findSecgroupByName(name string) (*entityv2.Secgroup, bool, error) {
	secgroups, err := r.provider.ListSecurityGroups(context.Background())
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

func (m *vngcloudLBBuilder) validateSecurityGroup(ids []string) error {

	listSecgroups, sdkErr := m.provider.ListSecurityGroups(m.context)
	if sdkErr != nil {
		return sdkErr
	}

	validIDs := make(map[string]bool)
	for _, secgroup := range listSecgroups.Items {
		validIDs[secgroup.Id] = true
	}

	invalid := make([]string, 0)
	for _, id := range ids {
		if _, ok := validIDs[id]; !ok {
			invalid = append(invalid, id)
		}
	}

	if len(invalid) > 0 {
		return errs.NewNoNeedRequeue(fmt.Sprintf("security group IDs: %v are invalid", strings.Join(invalid, ",")))
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------------------------

/*
- mode auto create secgroup:
  - check default exist, if not return
  - remove default from all server in target node
  - if default not in use, delete it

- mode manage secgroup:
  - if default exist, remove default from all server in target node
  - if default not in use, delete it
  - remove list secgroups from all server in target node

- if default secgroup is exist, remove it from all server and delete it
- if mode is auto create secgroup, return
*/
func (r *vngcloudLBBuilder) EnsureDeleteSecurityGroups(oldBuilder OldModelBuilder) error {
	// find default secgroup
	defaultSecgroup, isExists, err := r.findSecgroupByName(r.GetLoadBalancerDefaultName())
	if err != nil {
		r.logger.Error("Fail to find default secgroup", err)
		return err
	}

	removeList := make([]string, 0)

	if isExists {
		removeList = append(removeList, defaultSecgroup.Id)
	}

	// add all secgroups in oldBuilder to removeList
	if !oldBuilder.IsCreateDefaultSecgroup() {
		removeList = append(removeList, oldBuilder.GetOldSecGroups()...)
	}

	// remove secgroups from all server in target node
	err = r.ensureDeleteAddNodesSG(removeList, []string{}, r.knownNodes)
	if err != nil {
		return err
	}

	// delete default secgroup if possible
	if isExists {
		// if default secgroup is not in use, delete it
		isInUse, err := r.getServerUseSecgroup(defaultSecgroup.Id)
		if err != nil {
			return err
		}
		if !isInUse {
			err = r.provider.DeleteSecurityGroup(context.Background(), defaultSecgroup.Id)
			if err != nil {
				r.logger.Error("Fail to delete default secgroup", err)
				return err
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------------------------------------------------

// ensure delete old secgroups and add new secgroups to nodes
func (r *vngcloudLBBuilder) ensureDeleteAddNodesSG(oldSecgroups, newSecgroups []string, nodes []*corev1.Node) error {
	err := r.validateSecurityGroup(newSecgroups)
	if err != nil {
		return err
	}

	// get instance id of all servers
	instanceIDs := GetListProviderID(nodes)
	for _, instanceID := range instanceIDs {
		err := r.ensureSecgroupForInstance(instanceID, oldSecgroups, newSecgroups)
		if err != nil {
			r.logger.Error("Fail to ensure secgroup for instance: ", err)
			return err
		}
	}

	// should wait until the server is ready because list secgroup of server is not immediately updated
	for _, instanceID := range instanceIDs {
		err := r.provider.WaitForServerActive(r.context, instanceID)
		if err != nil {
			r.logger.Error("Fail to wait for server active: ", err)
			return err
		}
	}

	return nil
}

// ensure secgroup for instance, remove old secgroups and add new secgroups
func (m *vngcloudLBBuilder) ensureSecgroupForInstance(instanceID string, oldSecgroups, newSecgroups []string) error {
	// get security groups of instance
	instance, err := m.provider.GetServerByID(context.Background(), instanceID)
	if err != nil {
		m.logger.Error("Fail to get instance", err)
		return err
	}

	m.logger.Infof("Ensure security groups for instance %s", instanceID)

	currentSecgroups := make([]string, 0)
	for _, secgroup := range instance.SecGroups {
		currentSecgroups = append(currentSecgroups, secgroup.Uuid)
	}

	newSecgroups, isNeedUpdate := mergeStringArray(m.context, currentSecgroups, oldSecgroups, newSecgroups)
	if !isNeedUpdate {
		m.logger.Infof("No need to update security groups for instance: %v", instanceID)
		return nil
	}

	// update security groups of instance
	m.logger.Infof("Update security groups of instance %s: %v -> %v", instanceID, currentSecgroups, newSecgroups)
	_, err = m.provider.UpdateSecGroupsOfServer(context.Background(), instanceID, newSecgroups)
	if err != nil {
		m.logger.Error("Fail to update security groups of instance", err)
		return err
	}

	return nil
}

// return true if secgroup is in use
func (m *vngcloudLBBuilder) getServerUseSecgroup(secgroupID string) (bool, error) {
	listServer, err := m.provider.ListServerBySecgroupID(m.context, secgroupID)
	if err != nil {
		m.logger.Error("Fail to list server by secgroup", err)
		return true, err
	}

	if listServer == nil || listServer.Items == nil {
		listServer = &entityv2.ListServers{
			Items: []*entityv2.Server{},
		}
	}

	return len(listServer.Items) > 0, nil
}

func (m *vngcloudLBBuilder) compareSecgroupRule(current []*entityv2.SecgroupRule, new []*secGroupRuleBuilderType) ([]*entityv2.SecgroupRule, []*secGroupRuleBuilderType, error) {
	// get only ingress rules
	currentIngressRules := make([]*entityv2.SecgroupRule, 0)
	for _, rule := range current {
		if strings.EqualFold(rule.Direction, string(networkv2.SecgroupRuleDirectionIngress)) {
			currentIngressRules = append(currentIngressRules, rule)
		}
	}

	needDelete := make([]*entityv2.SecgroupRule, 0)
	needCreate := make([]*secGroupRuleBuilderType, 0)

	// mark all rule not in use
	ruleInUse := make(map[string]bool)
	for _, rule := range currentIngressRules {
		ruleInUse[rule.Id] = false
	}

	// check if the rule is in new
	for _, rule := range new {
		found := false
		for _, currentRule := range currentIngressRules {
			if rule.Description == currentRule.Description &&
				strings.EqualFold(string(rule.Direction), currentRule.Direction) &&
				strings.EqualFold(string(rule.EtherType), currentRule.EtherType) &&
				strings.EqualFold(string(rule.Protocol), currentRule.Protocol) &&
				rule.PortRangeMax == currentRule.PortRangeMax &&
				rule.PortRangeMin == currentRule.PortRangeMin &&
				rule.RemoteIPPrefix == currentRule.RemoteIPPrefix {

				ruleInUse[currentRule.Id] = true
				found = true
				break
			}
		}

		if !found {
			needCreate = append(needCreate, rule)
		}
	}

	// check if the rule is not in use
	for _, rule := range currentIngressRules {
		if !ruleInUse[rule.Id] {
			needDelete = append(needDelete, rule)
		}
	}

	return needDelete, needCreate, nil
}
