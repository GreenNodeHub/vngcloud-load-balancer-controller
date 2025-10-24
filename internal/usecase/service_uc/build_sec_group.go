package service_uc

import (
	"context"
	"fmt"
	"slices"

	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// buildIsAutoCreateSecGroup return
// - isAutoCreateSecurityGroup: if annotationn not exist, return true
// - secgroupIds: if annotation exist, return the list of secgroup ids
func (t *defaultModelBuildTask) buildIsAutoCreateSecGroup(_ context.Context) (isAutoCreateSecurityGroup bool, secgroupIds []string) {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixSecurityGroups, &option, t.service.Annotations)
	if !exist {
		return true, nil
	}
	return false, option
}

// buildDefaultSecurityGroupRule builds default security group rules
// based on the service ports and resolved member addresses.
func (t *defaultModelBuildTask) buildDefaultSecurityGroupRule(ctx context.Context, subnetCidr string) ([]v1alpha1.SecurityGroupRule, error) {
	secgroupRules := make([]v1alpha1.SecurityGroupRule, 0)
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(t.vlbConfig.Spec.TargetNodeLabels))),
	}

	for _, port := range t.service.Spec.Ports {
		// nodePort if target type is instance, targetPort if target type is ip
		var membersAddr []utils.EndpointAddress
		var err error
		if t.getTargetType(ctx) == "instance" {
			membersAddr, err = t.endpointResolver.ResolveNodePortEndpoints(ctx,
				utils.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return nil, err
			}

			// if cniMode is cilium native routing:
			// - port: is all pod ports
			// - CIDR: is all subnet CIDRs (lb->node or lb->node(not have pod)->node(have pod))
			if t.cniMode == utils.CiliumNativeRouting {
				podPorts, err := t.endpointResolver.GetListTargetPort(ctx, utils.NamespacedName(t.service), intstr.FromInt(int(port.Port)))
				if err != nil {
					return nil, err
				}
				allSubnetCidrs, err := t.getAllSubnetCidrs(ctx)
				if err != nil {
					return nil, err
				}
				for _, podPort := range podPorts {
					for _, cidr := range allSubnetCidrs {
						secgroupRules = append(secgroupRules, v1alpha1.SecurityGroupRule{
							Protocol:    t.coreProtocolToSecgroupProtocol(port.Protocol),
							FromPort:    int32(podPort),
							ToPort:      int32(podPort),
							CIDR:        cidr,
							Description: ptr.To("Allow other node access pod's port"), // TODO: improve description
						})
					}
				}
			}
		} else {
			membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
				utils.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return nil, err
			}
		}

		// if user set health check port:
		if healthCheckPort := t.buildHealthcheckPort(ctx); healthCheckPort != nil {
			secgroupRules = append(secgroupRules, v1alpha1.SecurityGroupRule{
				Protocol: t.coreProtocolToSecgroupProtocol(port.Protocol),
				FromPort: int32(*healthCheckPort),
				ToPort:   int32(*healthCheckPort),
				CIDR:     subnetCidr,
			})
		}

		// build secgroup rules from pool members
		for _, member := range membersAddr {
			// allow from subnet CIDR (subnet of load balancer) to node
			secgroupRules = append(secgroupRules, v1alpha1.SecurityGroupRule{
				Protocol: t.coreProtocolToSecgroupProtocol(port.Protocol),
				FromPort: int32(member.Port),
				ToPort:   int32(member.Port),
				CIDR:     subnetCidr,
			})
		}
	}

	secgroupRules = t.ensureSecgroupPING_UDP(ctx, secgroupRules)
	secgroupRules = t.ensureUniqueSecgroupRules(secgroupRules)
	return secgroupRules, nil
}

// ensureSecgroupPING_UDP ensures that for every UDP rule,
// there is also a corresponding ICMP rule to allow ping.
func (t *defaultModelBuildTask) ensureSecgroupPING_UDP(
	_ context.Context,
	rules []v1alpha1.SecurityGroupRule,
) []v1alpha1.SecurityGroupRule {
	newRules := make([]v1alpha1.SecurityGroupRule, len(rules))
	copy(newRules, rules)

	for _, rule := range rules {
		if rule.Protocol == networkv2.SecgroupRuleProtocolUDP {
			newRules = append(newRules, v1alpha1.SecurityGroupRule{
				Protocol:    networkv2.SecgroupRuleProtocolICMP,
				FromPort:    rule.FromPort,
				ToPort:      rule.ToPort,
				CIDR:        rule.CIDR,
				Description: ptr.To("Auto added ICMP rule for UDP"),
			})
		}
	}

	return newRules
}

// ensureUniqueSecgroupRules removes duplicate security group rules
// based on Protocol, FromPort, ToPort, and CIDR.
func (t *defaultModelBuildTask) ensureUniqueSecgroupRules(rules []v1alpha1.SecurityGroupRule) []v1alpha1.SecurityGroupRule {
	unique := make(map[string]bool)
	result := make([]v1alpha1.SecurityGroupRule, 0, len(rules))

	for _, rule := range rules {
		key := fmt.Sprintf("%s-%d-%d-%s", rule.Protocol, rule.FromPort, rule.ToPort, rule.CIDR)
		if !unique[key] {
			unique[key] = true
			result = append(result, rule)
		}
	}

	return result
}

func (t *defaultModelBuildTask) coreProtocolToSecgroupProtocol(protocol corev1.Protocol) networkv2.SecgroupRuleProtocol {
	switch protocol {
	case corev1.ProtocolTCP:
		return networkv2.SecgroupRuleProtocolTCP
	case corev1.ProtocolUDP:
		return networkv2.SecgroupRuleProtocolUDP
	default:
		return networkv2.SecgroupRuleProtocolTCP
	}
}

func (t *defaultModelBuildTask) getAllSubnetCidrs(ctx context.Context) ([]string, error) {
	subnetCidrs := make([]string, 0)

	nodes := &corev1.NodeList{}
	err := t.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		t.logger.Errorf("failed to list nodes: %v", err)
		return nil, err
	}

	providerIds := utils.GetListProviderIdFromNodeList(nodes)
	for _, providerId := range providerIds {
		_, _, _, cidr, err := t.vngcloudRepo.GetServerNetworkInfo(ctx, providerId)
		if err != nil {
			t.logger.Errorf("failed to get server network info for providerId %s: %v", providerId, err)
			return nil, err
		}
		if cidr == "" {
			continue
		}
		if slices.Contains(subnetCidrs, cidr) {
			continue
		}
		subnetCidrs = append(subnetCidrs, cidr)
	}

	return subnetCidrs, nil
}

// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
