package service_uc

import (
	"context"
	"fmt"
	"slices"

	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// buildIsAutoCreateSecGroup returns
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
func (t *defaultModelBuildTask) buildDefaultSecurityGroupRule(ctx context.Context, subnetCidr string, targetNodeLabels map[string]string) ([]v1alpha1.NodeSecurityGroupRule, error) {
	secgroupRules := make([]v1alpha1.NodeSecurityGroupRule, 0)
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(targetNodeLabels))),
	}

	for _, port := range t.service.Spec.Ports {
		// nodePort if target type is instance, targetPort if target type is ip
		var membersAddr []utils.EndpointAddress
		var err error
		if t.getTargetType(ctx) == domain.TargetTypeInstance {
			membersAddr, err = t.endpointResolver.ResolveNodePortEndpoints(ctx,
				k8s.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return nil, err
			}

			// if cniMode is cilium native routing:
			// - port: is all pod ports
			// - CIDR: is all subnet CIDRs (lb->node or lb->node(not have pod)->node(have pod))
			cniMode, err := t.cniDetector.DetectCNIType(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to detect CNI type: %v", err)
			}
			if cniMode == utils.CiliumNativeRouting {
				podPorts, err := t.endpointResolver.GetListTargetPort(ctx, k8s.NamespacedName(t.service), intstr.FromInt(int(port.Port)))
				if err != nil {
					return nil, err
				}
				allSubnetCidrs, err := t.getAllSubnetCidrs(ctx)
				if err != nil {
					return nil, err
				}
				for _, podPort := range podPorts {
					for _, cidr := range allSubnetCidrs {
						secgroupRules = append(secgroupRules, v1alpha1.NodeSecurityGroupRule{
							Protocol:    t.coreProtocolToSecgroupProtocol(port.Protocol),
							FromPort:    int32(podPort),
							ToPort:      int32(podPort),
							CIDR:        cidr,
							Description: "Allow other node access pod port - Cilium native",
							Direction:   networkv2.SecgroupRuleDirectionIngress,
							EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
						})
					}
				}
			}
		} else {
			membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
				k8s.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return nil, err
			}
		}

		// if user set health check port:
		if healthCheckPort := t.buildHealthcheckPort(ctx); healthCheckPort != nil {
			secgroupRules = append(secgroupRules, v1alpha1.NodeSecurityGroupRule{
				Protocol:    t.coreProtocolToSecgroupProtocol(port.Protocol),
				FromPort:    int32(*healthCheckPort),
				ToPort:      int32(*healthCheckPort),
				CIDR:        subnetCidr,
				Description: "Allow user custom health check port",
				Direction:   networkv2.SecgroupRuleDirectionIngress,
				EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
			})
		}

		// build secgroup rules from pool members
		for _, member := range membersAddr {
			// allow from subnet CIDR (subnet of load balancer) to node
			secgroupRules = append(secgroupRules, v1alpha1.NodeSecurityGroupRule{
				Protocol:    t.coreProtocolToSecgroupProtocol(port.Protocol),
				FromPort:    int32(member.Port),
				ToPort:      int32(member.Port),
				CIDR:        subnetCidr,
				Description: fmt.Sprintf("Allow load balancer access to port %d", member.Port),
				Direction:   networkv2.SecgroupRuleDirectionIngress,
				EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
			})
		}
	}

	secgroupRules = t.ensureSecgroupPING_UDP(ctx, secgroupRules)
	secgroupRules = t.ensureDefaultEngressSecgroupRule(ctx, secgroupRules)
	secgroupRules = t.ensureUniqueSecgroupRules(secgroupRules)

	// Sort rules to ensure deterministic ordering
	// This prevents unnecessary reconciliation loops caused by array order changes
	v1alpha1.SortSecurityGroupRules(secgroupRules)
	return secgroupRules, nil
}

// ensureDefaultEngressSecgroupRule ensures default egress rule to allow all outbound traffic
func (t *defaultModelBuildTask) ensureDefaultEngressSecgroupRule(
	_ context.Context,
	rules []v1alpha1.NodeSecurityGroupRule,
) []v1alpha1.NodeSecurityGroupRule {
	newRules := make([]v1alpha1.NodeSecurityGroupRule, len(rules))
	copy(newRules, rules)

	newRules = append(newRules, v1alpha1.NodeSecurityGroupRule{
		Protocol:    networkv2.SecgroupRuleProtocolAll,
		FromPort:    0,
		ToPort:      65535,
		CIDR:        "0.0.0.0/0",
		Direction:   networkv2.SecgroupRuleDirectionEgress,
		Description: "Default egress security group rule for IPv4",
		EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
	})
	newRules = append(newRules, v1alpha1.NodeSecurityGroupRule{
		Protocol:    networkv2.SecgroupRuleProtocolAll,
		FromPort:    0,
		ToPort:      65535,
		CIDR:        "::/0",
		Direction:   networkv2.SecgroupRuleDirectionEgress,
		Description: "Default egress security group rule for IPv6",
		EtherType:   networkv2.SecgroupRuleEtherTypeIPv6,
	})

	return newRules
}

// ensureSecgroupPING_UDP ensures that for every UDP rule,
// there is also a corresponding ICMP rule to allow ping.
func (t *defaultModelBuildTask) ensureSecgroupPING_UDP(
	_ context.Context,
	rules []v1alpha1.NodeSecurityGroupRule,
) []v1alpha1.NodeSecurityGroupRule {
	newRules := make([]v1alpha1.NodeSecurityGroupRule, len(rules))
	copy(newRules, rules)

	for _, rule := range rules {
		if rule.Protocol == networkv2.SecgroupRuleProtocolUDP {
			newRules = append(newRules, v1alpha1.NodeSecurityGroupRule{
				Protocol:    networkv2.SecgroupRuleProtocolICMP,
				FromPort:    1,   // ICMP layer 3, no port, only type and code
				ToPort:      255, // ICMP layer 3, no port, only type and code
				CIDR:        rule.CIDR,
				Description: "Allow ICMP for health check UDP port",
				Direction:   rule.Direction,
				EtherType:   rule.EtherType,
			})
		}
	}

	return newRules
}

// ensureUniqueSecgroupRules removes duplicate security group rules
// based on Protocol, FromPort, ToPort, and CIDR.
func (t *defaultModelBuildTask) ensureUniqueSecgroupRules(rules []v1alpha1.NodeSecurityGroupRule) []v1alpha1.NodeSecurityGroupRule {
	unique := make(map[string]bool)
	result := make([]v1alpha1.NodeSecurityGroupRule, 0, len(rules))

	for _, rule := range rules {
		key := fmt.Sprintf("%s-%d-%d-%s-%s-%s",
			rule.Protocol,
			rule.FromPort,
			rule.ToPort,
			rule.CIDR,
			rule.Direction,
			rule.EtherType)
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
