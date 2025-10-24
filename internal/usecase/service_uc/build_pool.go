package service_uc

import (
	"context"
	"strings"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func (t *defaultModelBuildTask) buildPoolsAndListeners(ctx context.Context) error {
	allPools := make([]v1alpha1.Pool, 0)
	allListeners := make([]v1alpha1.Listener, 0)

	ports := t.service.Spec.Ports
	if len(ports) <= 0 {
		return nil
	}

	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(t.vlbConfig.Spec.TargetNodeLabels))),
	}
	defaultPoolAlgorithm := t.buildPoolAlgorithm(ctx)
	defaultHealthcheckPort := t.buildHealthcheckPort(ctx)
	defaultIdleTimeoutClient := t.buildIdleTimeoutClient(ctx)
	defaultIdleTimeoutMember := t.buildIdleTimeoutMember(ctx)
	defaultIdleTimeoutConnection := t.buildIdleTimeoutConnection(ctx)
	defaultAllowedCidrs := t.buildInboundCIDRs(ctx)
	defaultEnableProxyProtocol := t.buildEnableProxyProtocol(ctx)

	// Build pool and listener
	for _, port := range ports {
		// nodePort if target type is instance, targetPort if target type is ip
		var membersAddr []utils.EndpointAddress
		var err error
		if t.getTargetType(ctx) == "instance" {
			membersAddr, err = t.endpointResolver.ResolveNodePortEndpoints(ctx,
				utils.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				t.logger.Errorf("failed to resolve node port endpoints: %v", err)
				return err
			}
		} else {
			membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
				utils.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				t.logger.Errorf("failed to resolve pod endpoints: %v", err)
				return err
			}
		}

		// build pool members
		poolMembers := make([]v1alpha1.PoolMember, 0)
		for _, member := range membersAddr {
			poolMember := v1alpha1.PoolMember{
				IP:          member.IP,
				Port:        member.Port,
				MonitorPort: &(member.Port), // default monitor port = member port, can be override by annotation
				Name:        member.Name,
			}
			if defaultHealthcheckPort != nil {
				poolMember.MonitorPort = defaultHealthcheckPort
			}
			poolMembers = append(poolMembers, poolMember)
		}

		newPool := v1alpha1.Pool{
			Name:      t.nameHelper.GenL4PoolName(port, string(port.Protocol)),
			Protocol:  loadbalancerv2.PoolProtocol(port.Protocol),
			Members:   poolMembers,
			Algorithm: defaultPoolAlgorithm,
		}
		for _, name := range defaultEnableProxyProtocol {
			if (name == "*" || name == port.Name) && port.Protocol == corev1.ProtocolTCP {
				newPool.Protocol = loadbalancerv2.PoolProtocolProxy
				newPool.Name = t.nameHelper.GenL4PoolName(port, string(loadbalancerv2.PoolProtocolProxy))
				break
			}
		}
		allPools = append(allPools, newPool)

		newListener := v1alpha1.Listener{
			Name:              t.nameHelper.GenL4ListenerName(port),
			Protocol:          loadbalancerv2.ListenerProtocol(port.Protocol),
			ProtocolPort:      int32(port.Port), // TODO
			DefaultPoolName:   &newPool.Name,
			TimeoutClient:     defaultIdleTimeoutClient,
			TimeoutMember:     defaultIdleTimeoutMember,
			TimeoutConnection: defaultIdleTimeoutConnection,
			AllowedCidrs:      defaultAllowedCidrs,
		}
		allListeners = append(allListeners, newListener)
	}

	t.vlbConfig.Spec.Pools = allPools
	t.vlbConfig.Spec.Listeners = allListeners
	return nil
}

func (t *defaultModelBuildTask) getTargetType(_ context.Context) string {
	// TODO: store somewhere to avoid parsing again and again
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixTargetType, &option, t.service.Annotations)
	if option == "ip" || option == "instance" {
		return option
	}
	return "instance"
}

func (t *defaultModelBuildTask) buildHealthcheckPort(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckPort, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixHealthcheckPort, 0)
		return nil
	}
	if optionsInt64 <= 0 || optionsInt64 > 65535 {
		t.logger.Warnf("Invalid annotation \"%s\" value %d, must be in range 1-65535, using default value %d",
			annotations.SuffixHealthcheckPort, optionsInt64, 0)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolAlgorithm(_ context.Context) *loadbalancerv2.PoolAlgorithm {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixPoolAlgorithm, &option, t.service.Annotations)
	if !exist {
		return nil
	}

	switch option {
	case string(loadbalancerv2.PoolAlgorithmLeastConn),
		string(loadbalancerv2.PoolAlgorithmRoundRobin),
		string(loadbalancerv2.PoolAlgorithmSourceIP):
		return ptr.To(loadbalancerv2.PoolAlgorithm(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixPoolAlgorithm,
			loadbalancerv2.PoolAlgorithmLeastConn,
			loadbalancerv2.PoolAlgorithmRoundRobin,
			loadbalancerv2.PoolAlgorithmSourceIP)
	}
	return nil
}

func (t *defaultModelBuildTask) buildIdleTimeoutClient(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutClient, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutClient, 0)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildIdleTimeoutMember(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutMember, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutMember, 0)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildIdleTimeoutConnection(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutConnection, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer, using default value %d",
			annotations.SuffixIdleTimeoutConnection, 0)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildInboundCIDRs(_ context.Context) *string {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixInboundCIDRs, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return ptr.To(strings.Join(option, ","))
}

func (t *defaultModelBuildTask) buildEnableProxyProtocol(_ context.Context) []string {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixEnableProxyProtocol, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return option
}

// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
