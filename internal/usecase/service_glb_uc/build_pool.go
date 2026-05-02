package service_glb_uc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func (t *defaultModelBuildTask) buildPoolsAndListeners(ctx context.Context, targetNodeLabels map[string]string) ([]v1alpha1.GlobalPool, []v1alpha1.GlobalListener, error) {
	allPools := make([]v1alpha1.GlobalPool, 0)
	allListeners := make([]v1alpha1.GlobalListener, 0)

	ports := t.service.Spec.Ports
	if len(ports) <= 0 {
		return nil, nil, nil
	}

	defaultIdleTimeoutClient := t.buildIdleTimeoutClient(ctx)
	defaultIdleTimeoutMember := t.buildIdleTimeoutMember(ctx)
	defaultIdleTimeoutConnection := t.buildIdleTimeoutConnection(ctx)
	defaultAllowedCidrs := t.buildInboundCIDRs(ctx)

	for _, port := range ports {
		newPool, err := t.buildPool(ctx, port, targetNodeLabels)
		if err != nil {
			t.logger.Errorf("failed to build pool for port %d: %v", port.Port, err)
			return nil, nil, err
		}

		allPools = append(allPools, *newPool)

		newListener := v1alpha1.GlobalListener{
			Name:              t.genListenerName(port),
			Protocol:          t.getListenerProtocol(port.Protocol),
			ProtocolPort:      int(port.Port),
			DefaultPoolName:   &newPool.Name,
			TimeoutClient:     defaultIdleTimeoutClient,
			TimeoutMember:     defaultIdleTimeoutMember,
			TimeoutConnection: defaultIdleTimeoutConnection,
			AllowedCidrs:      defaultAllowedCidrs,
		}
		allListeners = append(allListeners, newListener)
	}

	// Sort pools and listeners by name to ensure deterministic ordering
	sort.Slice(allPools, func(i, j int) bool {
		return allPools[i].Name < allPools[j].Name
	})
	sort.Slice(allListeners, func(i, j int) bool {
		return allListeners[i].Name < allListeners[j].Name
	})

	return allPools, allListeners, nil
}

func (t *defaultModelBuildTask) buildPool(ctx context.Context, port corev1.ServicePort, targetNodeLabels map[string]string) (*v1alpha1.GlobalPool, error) {
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(targetNodeLabels))),
	}
	defaultHealthcheckPort := t.buildHealthcheckPort(ctx)

	// nodePort if target type is instance, targetPort if target type is ip
	var membersAddr []utils.EndpointAddress
	var err error
	if t.getTargetType(ctx) == domain.TargetTypeInstance {
		membersAddr, err = t.endpointResolver.ResolveNodePortEndpoints(ctx,
			k8s.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
		if err != nil {
			t.logger.Errorf("failed to resolve node port endpoints: %v", err)
			return nil, err
		}
	} else {
		membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
			k8s.NamespacedName(t.service), intstr.FromInt(int(port.Port)), resolveOpts...)
		if err != nil {
			t.logger.Errorf("failed to resolve pod endpoints: %v", err)
			return nil, err
		}
	}

	// Build pool members
	globalMembers := make([]v1alpha1.GlobalMember, 0)
	for _, member := range membersAddr {
		monitorPort := member.Port
		if defaultHealthcheckPort != nil {
			monitorPort = *defaultHealthcheckPort
		}
		globalMember := v1alpha1.GlobalMember{
			Address:     member.IP,
			Port:        member.Port,
			MonitorPort: &monitorPort,
			Name:        member.Name,
			SubnetID:    t.defaultSubnetId,
		}
		globalMembers = append(globalMembers, globalMember)
	}

	// Sort global members by Address:Port for deterministic ordering
	sort.Slice(globalMembers, func(i, j int) bool {
		if globalMembers[i].Address != globalMembers[j].Address {
			return globalMembers[i].Address < globalMembers[j].Address
		}
		return globalMembers[i].Port < globalMembers[j].Port
	})

	// Wrap members in a GlobalPoolMember (region/VPC grouping)
	poolMembers := []v1alpha1.GlobalPoolMember{}
	if len(globalMembers) > 0 {
		poolMembers = append(poolMembers, v1alpha1.GlobalPoolMember{
			Name:    fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId),
			Region:  t.defaultRegion,
			VpcId:   t.defaultNetworkId,
			Type:    global.GlobalPoolMemberTypePrivate,
			Members: globalMembers,
		})
	}

	// Build healthcheck
	healthMonitor := v1alpha1.GlobalPoolHealthMonitor{
		Protocol:           t.buildPoolHealthCheckProtocol(ctx, port.Protocol),
		HealthyThreshold:   t.buildPoolHealthyThresholdCount(ctx),
		UnhealthyThreshold: t.buildPoolUnhealthyThresholdCount(ctx),
		Interval:           t.buildPoolHealthcheckIntervalSeconds(ctx),
		Timeout:            t.buildPoolHealthcheckTimeoutSeconds(ctx),
		HealthCheckMethod:  nil,
		HealthCheckPath:    nil,
		SuccessCode:        nil,
		HttpVersion:        nil,
		DomainName:         nil,
	}
	if healthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTP ||
		healthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTPs {
		healthMonitor.HealthCheckMethod = t.buildAnnotationHealthcheckHttpMethod(ctx)
		healthMonitor.HealthCheckPath = t.buildAnnotationHealthcheckPath(ctx)
		healthMonitor.SuccessCode = t.buildAnnotationSuccessCodes(ctx)
		healthMonitor.HttpVersion = t.buildAnnotationHealthcheckHttpVersion(ctx)
		healthMonitor.DomainName = t.buildAnnotationHealthcheckHttpDomainName(ctx)
	}

	newPool := v1alpha1.GlobalPool{
		Name:          t.genPoolName(port, string(port.Protocol)),
		Protocol:      t.getPoolProtocol(port.Protocol),
		PoolMembers:   poolMembers,
		Algorithm:     t.buildPoolAlgorithm(ctx),
		HealthMonitor: healthMonitor,
	}
	for _, name := range t.buildEnableProxyProtocol(ctx) {
		if (name == "*" || name == port.Name) && port.Protocol == corev1.ProtocolTCP {
			newPool.Protocol = global.GlobalPoolProtocolProxy
			newPool.Name = t.genPoolName(port, string(global.GlobalPoolProtocolProxy))
			break
		}
	}
	return &newPool, nil
}

// genPoolName returns a deterministic pool name based on port number.
func (t *defaultModelBuildTask) genPoolName(port corev1.ServicePort, protocol string) string {
	return fmt.Sprintf("pool-%d-%s", port.Port, strings.ToLower(protocol))
}

// getPoolProtocol returns the GLB pool protocol (TCP or Proxy; no UDP support).
func (t *defaultModelBuildTask) getPoolProtocol(_ corev1.Protocol) global.GlobalPoolProtocol {
	return global.GlobalPoolProtocolTCP
}

// getTargetType returns the target type for pool members.
// ClusterIP services force TargetTypeIP (no NodePort available).
func (t *defaultModelBuildTask) getTargetType(_ context.Context) domain.TargetType {
	// Force target type to IP for ClusterIP services (no NodePort available)
	if t.service.Spec.Type == corev1.ServiceTypeClusterIP {
		return domain.TargetTypeIP
	}

	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBTargetType, &option, t.service.Annotations)
	if option == string(domain.TargetTypeIP) || option == string(domain.TargetTypeInstance) {
		return domain.TargetType(option)
	}
	return domain.TargetTypeInstance
}

func (t *defaultModelBuildTask) buildHealthcheckPort(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBHealthcheckPort, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBHealthcheckPort)
		return nil
	}
	if optionsInt64 <= 0 || optionsInt64 > 65535 {
		t.logger.Warnf("Invalid annotation \"%s\" value %d, must be in range 1-65535.", annotations.SuffixGLBHealthcheckPort, optionsInt64)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolAlgorithm(_ context.Context) *global.GlobalPoolAlgorithm {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBPoolAlgorithm, &option, t.service.Annotations)
	if !exist {
		return nil
	}

	switch option {
	case string(global.GlobalPoolAlgorithmRoundRobin),
		string(global.GlobalPoolAlgorithmLeastConn),
		string(global.GlobalPoolAlgorithmSourceIP):
		return ptr.To(global.GlobalPoolAlgorithm(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixGLBPoolAlgorithm,
			global.GlobalPoolAlgorithmRoundRobin,
			global.GlobalPoolAlgorithmLeastConn,
			global.GlobalPoolAlgorithmSourceIP)
	}
	return nil
}

func (t *defaultModelBuildTask) buildIdleTimeoutClient(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBIdleTimeoutClient, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBIdleTimeoutClient)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildIdleTimeoutMember(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBIdleTimeoutMember, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBIdleTimeoutMember)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildIdleTimeoutConnection(_ context.Context) *int32 {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBIdleTimeoutConnection, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBIdleTimeoutConnection)
		return nil
	}
	return ptr.To(int32(optionsInt64))
}

func (t *defaultModelBuildTask) buildInboundCIDRs(_ context.Context) *string {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixGLBInboundCIDRs, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return ptr.To(strings.Join(option, ","))
}

func (t *defaultModelBuildTask) buildEnableProxyProtocol(_ context.Context) []string {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixGLBEnableProxyProtocol, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildPoolHealthCheckProtocol(_ context.Context, _ corev1.Protocol) global.GlobalPoolHealthCheckProtocol {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBHealthcheckProtocol, &option, t.service.Annotations)
	if !exist {
		return global.GlobalPoolHealthCheckProtocolTCP
	}

	switch strings.TrimSpace(strings.ToUpper(option)) {
	case string(global.GlobalPoolHealthCheckProtocolHTTP):
		return global.GlobalPoolHealthCheckProtocolHTTP
	case string(global.GlobalPoolHealthCheckProtocolHTTPs):
		return global.GlobalPoolHealthCheckProtocolHTTPs
	}

	return global.GlobalPoolHealthCheckProtocolTCP
}

func (t *defaultModelBuildTask) buildPoolHealthyThresholdCount(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBHealthyThresholdCount, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBHealthyThresholdCount)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolUnhealthyThresholdCount(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBUnhealthyThresholdCount, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBUnhealthyThresholdCount)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolHealthcheckIntervalSeconds(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBHealthcheckIntervalSeconds, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBHealthcheckIntervalSeconds)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolHealthcheckTimeoutSeconds(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixGLBHealthcheckTimeoutSeconds, &optionsInt64, t.service.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.", annotations.SuffixGLBHealthcheckTimeoutSeconds)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpMethod(_ context.Context) *global.GlobalPoolHealthCheckMethod {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBHealthcheckHttpMethod, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	switch option {
	case string(global.GlobalPoolHealthCheckMethodGET),
		string(global.GlobalPoolHealthCheckMethodPUT),
		string(global.GlobalPoolHealthCheckMethodPOST):
		return ptr.To(global.GlobalPoolHealthCheckMethod(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixGLBHealthcheckHttpMethod,
			global.GlobalPoolHealthCheckMethodGET,
			global.GlobalPoolHealthCheckMethodPUT,
			global.GlobalPoolHealthCheckMethodPOST)
		return nil
	}
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckPath(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBHealthcheckPath, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationSuccessCodes(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBSuccessCodes, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpVersion(_ context.Context) *global.GlobalPoolHealthCheckHttpVersion {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBHealthcheckHttpVersion, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	switch option {
	case string(global.GlobalPoolHealthCheckHttpVersionHttp1),
		string(global.GlobalPoolHealthCheckHttpVersionHttp1Minor1):
		return ptr.To(global.GlobalPoolHealthCheckHttpVersion(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"",
			annotations.SuffixGLBHealthcheckHttpVersion,
			global.GlobalPoolHealthCheckHttpVersionHttp1,
			global.GlobalPoolHealthCheckHttpVersionHttp1Minor1)
	}
	return nil
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpDomainName(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBHealthcheckHttpDomainName, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}
