package ingress_uc

import (
	"context"
	"strings"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func (t *defaultModelBuildTask) buildPoolsAndListeners(ctx context.Context, targetNodeLabels map[string]string) ([]v1alpha1.Pool, []v1alpha1.Listener, error) {
	allPools := make([]v1alpha1.Pool, 0)
	appendPool := func(newPool v1alpha1.Pool) {
		// check if pool name already exist
		for _, existingPool := range allPools {
			if existingPool.Name == newPool.Name {
				return
			}
		}
		allPools = append(allPools, newPool)
	}

	allListeners := make([]v1alpha1.Listener, 0)

	// build default backend pool
	isHaveDefaultBackend := t.ingress.Spec.DefaultBackend != nil
	var defaultPool *v1alpha1.Pool
	var err error
	if isHaveDefaultBackend {
		t.logger.Debugf("ingress %s/%s has default backend, building default pool.", t.ingress.Namespace, t.ingress.Name)
		defaultPool, err = t.buildPool(ctx, t.ingress.Spec.DefaultBackend.Service, targetNodeLabels)
		if err != nil {
			return nil, nil, err
		}
		defaultPool.Name = domain.DEFAULT_NAME_DEFAULT_POOL
		appendPool(*defaultPool)
	}

	// build listener http
	isNeedHTTPListener := t.checkNeedHTTPListener(ctx)
	var httpListener *v1alpha1.Listener
	if isNeedHTTPListener {
		httpListener, err = t.buildListeners(ctx, false)
		if err != nil {
			return nil, nil, err
		}
		if isHaveDefaultBackend {
			httpListener.DefaultPoolName = &defaultPool.Name
		}
	}

	// build listener https
	var httpsListener *v1alpha1.Listener
	isValidToBuildHTTPSListener, err := t.isValidToBuildHTTPSListener(ctx)
	if err != nil {
		return nil, nil, err
	}
	if isValidToBuildHTTPSListener {
		httpsListener, err = t.buildListeners(ctx, true)
		if err != nil {
			return nil, nil, err
		}
		if isHaveDefaultBackend {
			httpsListener.DefaultPoolName = &defaultPool.Name
		}
	}

	// which host using tls
	hostUsingTLS := make(map[string]bool)
	for _, tls := range t.ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			hostUsingTLS[host] = true
		}
	}

	// build policy
	for ruleIndex, rule := range t.ingress.Spec.Rules {
		_, isHttpsListener := hostUsingTLS[rule.Host]

		for pathIndex, path := range rule.HTTP.Paths {

			// build policy
			policyName := t.nameHelper.GenL7PolicyName(isHttpsListener, ruleIndex, pathIndex)
			newPolicy, err := t.buildPolicyByPath(ctx, rule.Host, policyName, &path)
			if err != nil {
				return nil, nil, err
			}

			// check if action is redirect to pool
			if newPolicy.Action == loadbalancerv2.PolicyActionREDIRECTTOPOOL {
				// build pool
				newPool, err := t.buildPool(ctx, path.Backend.Service, targetNodeLabels)
				if err != nil {
					return nil, nil, err
				}
				appendPool(*newPool)

				// set policy refer to pool name
				newPolicy.RedirectPoolName = &newPool.Name
			}

			if isHttpsListener {
				httpsListener.Policies = append(httpsListener.Policies, *newPolicy)
			} else {
				httpListener.Policies = append(httpListener.Policies, *newPolicy)
			}
		}
	}

	if isNeedHTTPListener {
		allListeners = append(allListeners, *httpListener)
	}

	if isValidToBuildHTTPSListener {
		allListeners = append(allListeners, *httpsListener)
	}

	return allPools, allListeners, nil
}

// from serviceBackend include name and port, build the pool
func (t *defaultModelBuildTask) buildPool(ctx context.Context, service *networkingv1.IngressServiceBackend, targetNodeLabels map[string]string) (*v1alpha1.Pool, error) {
	// find service
	findService, err := t.k8sRepo.GetService(ctx, types.NamespacedName{Namespace: t.ingress.GetNamespace(), Name: service.Name})
	if err != nil {
		t.logger.Errorf("failed to get service %s/%s: %v", t.ingress.GetNamespace(), service.Name, err)
		return nil, err
	}

	// find service port
	var servicePort *corev1.ServicePort
	for _, port := range findService.Spec.Ports {
		port := port
		if service.Port.Name != "" && service.Port.Name == port.Name {
			servicePort = &port
			break
		} else if service.Port.Number == port.Port {
			servicePort = &port
			break
		}
	}
	if servicePort == nil {
		return nil, errs.NewNoNeedRequeue("service port not found")
	}

	// Get members address, nodeIP or podIP
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(targetNodeLabels))),
	}

	// nodePort if target type is instance, targetPort if target type is ip
	var membersAddr []utils.EndpointAddress
	if t.getTargetType(ctx) == domain.TargetTypeInstance {
		membersAddr, err = t.endpointResolver.ResolveNodePortEndpoints(ctx,
			types.NamespacedName{Namespace: t.ingress.GetNamespace(), Name: service.Name}, serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			t.logger.Errorf("failed to resolve node port endpoints: %v", err)
			return nil, err
		}
	} else {
		membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
			types.NamespacedName{Namespace: t.ingress.GetNamespace(), Name: service.Name}, serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			t.logger.Errorf("failed to resolve pod endpoints: %v", err)
			return nil, err
		}
	}

	defaultHealthcheckPort := t.buildHealthcheckPort(ctx)

	// build pool members
	poolMembers := make([]v1alpha1.PoolMember, 0)
	for _, member := range membersAddr {
		poolMember := v1alpha1.PoolMember{
			IP:          member.IP,
			Port:        member.Port,
			MonitorPort: member.Port, // default monitor port = member port, can be override by annotation
			Name:        member.Name,
		}
		if defaultHealthcheckPort != nil {
			poolMember.MonitorPort = *defaultHealthcheckPort
		}
		poolMembers = append(poolMembers, poolMember)
	}

	// build healthcheck
	healthMonitor := v1alpha1.PoolHealthMonitor{
		Protocol:           t.buildPoolHealthCheckProtocol(ctx, corev1.ProtocolTCP), // TODO review it
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
	// L7 support healthcheck TCP and HTTP
	if healthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolHTTP {

		healthMonitor.HealthCheckMethod = t.buildAnnotationHealthcheckHttpMethod(ctx)
		healthMonitor.HealthCheckPath = t.buildAnnotationHealthcheckPath(ctx)
		healthMonitor.SuccessCode = t.buildAnnotationSuccessCodes(ctx)
		healthMonitor.HttpVersion = t.buildAnnotationHealthcheckHttpVersion(ctx)
		healthMonitor.DomainName = t.buildAnnotationHealthcheckHttpDomainName(ctx)
	}

	newPool := v1alpha1.Pool{
		Name:          t.nameHelper.GenL7PoolName(service.Name, int(servicePort.Port)),
		Protocol:      loadbalancerv2.PoolProtocolHTTP,
		Members:       poolMembers,
		Algorithm:     t.buildPoolAlgorithm(ctx),
		HealthMonitor: healthMonitor,
		Stickiness:    t.buildPoolStickiness(ctx),
		TLSEncryption: t.buildPoolTLSEncryption(ctx),
	}

	return &newPool, nil
}

func (t *defaultModelBuildTask) getTargetType(_ context.Context) domain.TargetType {
	// TODO: store somewhere to avoid parsing again and again
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixTargetType, &option, t.ingress.Annotations)
	if option == string(domain.TargetTypeIP) || option == string(domain.TargetTypeInstance) {
		return domain.TargetType(option)
	}
	return domain.TargetTypeInstance
}

func (t *defaultModelBuildTask) buildHealthcheckPort(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckPort, &optionsInt64, t.ingress.Annotations)
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
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixPoolAlgorithm, &option, t.ingress.Annotations)
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
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutClient, &optionsInt64, t.ingress.Annotations)
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
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutMember, &optionsInt64, t.ingress.Annotations)
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
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixIdleTimeoutConnection, &optionsInt64, t.ingress.Annotations)
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
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixInboundCIDRs, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	return ptr.To(strings.Join(option, ","))
}

func (t *defaultModelBuildTask) buildPoolHealthCheckProtocol(_ context.Context, pPoolProtocol corev1.Protocol) loadbalancerv2.HealthCheckProtocol {
	switch pPoolProtocol {
	case corev1.ProtocolUDP:
		return loadbalancerv2.HealthCheckProtocolPINGUDP
	}

	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckProtocol, &option, t.ingress.Annotations)
	if !exist {
		return loadbalancerv2.HealthCheckProtocolTCP
	}

	switch strings.TrimSpace(strings.ToUpper(option)) {
	case string(loadbalancerv2.HealthCheckProtocolHTTP):
		return loadbalancerv2.HealthCheckProtocolHTTP
	case string(loadbalancerv2.HealthCheckProtocolHTTPs):
		return loadbalancerv2.HealthCheckProtocolHTTPs
	case string(loadbalancerv2.HealthCheckProtocolPINGUDP):
		return loadbalancerv2.HealthCheckProtocolPINGUDP
	}

	return loadbalancerv2.HealthCheckProtocolTCP
}

func (t *defaultModelBuildTask) buildPoolHealthyThresholdCount(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixHealthyThresholdCount, &optionsInt64, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.",
			annotations.SuffixHealthyThresholdCount)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolUnhealthyThresholdCount(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixUnhealthyThresholdCount, &optionsInt64, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.",
			annotations.SuffixUnhealthyThresholdCount)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolHealthcheckIntervalSeconds(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckIntervalSeconds, &optionsInt64, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.",
			annotations.SuffixHealthcheckIntervalSeconds)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolHealthcheckTimeoutSeconds(_ context.Context) *int {
	optionsInt64 := int64(0)
	exists, err := t.annotationParser.ParseInt64Annotation(annotations.SuffixHealthcheckTimeoutSeconds, &optionsInt64, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer.",
			annotations.SuffixHealthcheckTimeoutSeconds)
		return nil
	}
	return ptr.To(int(optionsInt64))
}

func (t *defaultModelBuildTask) buildPoolStickiness(_ context.Context) *bool {
	option := false
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableStickySession, &option, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean.",
			annotations.SuffixEnableStickySession)
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildPoolTLSEncryption(_ context.Context) *bool {
	option := false
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableTLSEncryption, &option, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean.",
			annotations.SuffixEnableTLSEncryption)
		return nil
	}
	return &option
}

// serviceBackendToIntOrString converts a ServiceBackendPort (Ingress) to an IntOrString
func serviceBackendToIntOrString(port networkingv1.ServiceBackendPort) intstr.IntOrString {
	if port.Name != "" {
		return intstr.FromString(port.Name)
	}
	return intstr.FromInt(int(port.Number))
}

// if have default backend,
// if have host not in tls,
// if have host = ""
func (t *defaultModelBuildTask) checkNeedHTTPListener(_ context.Context) bool {
	if t.ingress.Spec.DefaultBackend != nil {
		t.logger.Debugf("Need HTTP listener: has default backend")
		return true
	}
	for _, rule := range t.ingress.Spec.Rules {
		if rule.Host == "" {
			t.logger.Debugf("Need HTTP listener: has rule with empty host")
			return true
		}
	}
	tlsHosts := make(map[string]bool)
	for _, tls := range t.ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			tlsHosts[host] = true
		}
	}
	for _, rule := range t.ingress.Spec.Rules {
		if _, ok := tlsHosts[rule.Host]; !ok {
			t.logger.Debugf("Need HTTP listener: has rule with host not in tls")
			return true
		}
	}
	return false
}

func (t *defaultModelBuildTask) isValidToBuildHTTPSListener(ctx context.Context) (bool, error) {
	if len(t.ingress.Spec.TLS) == 0 {
		return false, nil
	}
	if len(t.buildAnnotationCertificateIds(ctx)) > 0 {
		return true, nil
	}
	for _, tls := range t.ingress.Spec.TLS {
		if tls.SecretName == "" {
			return true, errs.NewNoNeedRequeue("to use TLS, specific certIDs through annotation or secretName must be set")
		}
	}
	return true, nil
}

func (t *defaultModelBuildTask) buildAnnotationCertificateIds(_ context.Context) []string {
	option := []string{}
	exist := t.annotationParser.ParseStringSliceAnnotation(annotations.SuffixCertificateIDs, &option, t.ingress.Annotations)
	if !exist {
		return []string{}
	}
	return option
}
