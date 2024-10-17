package builder

import (
	"context"
	"slices"
	"strings"

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
	defaultSecgroup, isExists, err := r.findSecgroupByName(newBuilder.GetLoadBalancerDefaultName())
	if err != nil {
		r.logger.Error("Fail to find default secgroup", err)
		return err
	}

	// if default secgroup but newBuilder not use it, delete it
	if isExists && !newBuilder.IsCreateDefaultSecgroup() &&
		!slices.Contains(newBuilder.GetSecurityGroupIDs(), defaultSecgroup.Id) {
		// delete default secgroup
		err := r.provider.DeleteSecurityGroup(context.Background(), defaultSecgroup.Id)
		if err != nil {
			r.logger.Error("Fail to delete default secgroup", err)
			return err
		}
	}

	// get selector node
	var nodes []*corev1.Node
	if newBuilder.GetTargetType() == TargetTypeIP {
		nodes, err = newBuilder.GetNodeBySelector(map[string]string{})
	} else {
		nodes, err = newBuilder.GetNodeBySelector(newBuilder.GetTargetNodeLabels())
	}
	if err != nil {
		r.logger.Error("Fail to get node by selector", err)
		return err
	}

	// ensure all server using the secgroups, which are in the annotation
	if !newBuilder.IsCreateDefaultSecgroup() {
		r.ensureServerHaveSecurityGroup(oldBuilder.GetOldSecGroups(), newBuilder.GetSecurityGroupIDs(), nodes)
		return nil
	}

	// if default secgroup not exists, create it
	if !isExists {
		_secG, err := r.provider.CreateSecurityGroup(context.Background(), newBuilder.GetLoadBalancerDefaultName(),
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
		r.logger.Debugf("   - current rule %v", rule)
	}

	// ensure secgroup rules
	newBuilder.EnsureSecgroupPING_UDP()
	needDelete, needCreate, err := VNGHelper.CompareSecgroupRule(defaultSecgroupRules.Items, newBuilder.GetListDefaultSecgroupRules())
	if err != nil {
		r.logger.Error("Fail to compare secgroup rules", err)
		return err
	}

	for _, rule := range needDelete {
		r.logger.Debugf("   - delete rule: %v", rule)
	}
	for _, rule := range needCreate {
		r.logger.Debugf("   - create rule: %v", rule)
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
	err = r.ensureServerHaveSecurityGroup(oldBuilder.GetOldSecGroups(), []string{defaultSecgroup.Id}, nodes)
	if err != nil {
		r.logger.Error("Fail to ensure server have secgroup", err)
		return err
	}

	return nil
}

func (r *vngcloudLBBuilder) ensureServerHaveSecurityGroup(oldSecgroups, newSecgroups []string, nodes []*corev1.Node) error {
	err := r.validateSecurityGroup(newSecgroups)
	if err != nil {
		return err
	}

	// get instance id of all servers
	instanceIDs := VNGHelper.GetListProviderID(nodes)
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
		m.logger.Errorf("Security group IDs %v are invalid", strings.Join(invalid, ","))
		return errs.ErrorSecurityGroupNotFound
	}
	return nil
}

func (m *vngcloudLBBuilder) ensureSecgroupForInstance(instanceID string, oldSecgroups, newSecgroups []string) error {
	// get security groups of instance
	instance, err := m.provider.GetServerByID(context.Background(), instanceID)
	if err != nil {
		m.logger.Error("Fail to get instance", err)
		return err
	}

	currentSecgroups := make([]string, 0)
	for _, secgroup := range instance.SecGroups {
		currentSecgroups = append(currentSecgroups, secgroup.Uuid)
	}

	newSecgroups, isNeedUpdate := VNGHelper.MergeStringArray(m.context, currentSecgroups, oldSecgroups, newSecgroups)
	if !isNeedUpdate {
		m.logger.Infof("No need to update security groups for instance: %v", instanceID)
		return nil
	}

	// update security groups of instance
	_, err = m.provider.UpdateSecGroupsOfServer(context.Background(), instanceID, newSecgroups)
	if err != nil {
		m.logger.Error("Fail to update security groups of instance", err)
		return err
	}

	return nil
}
