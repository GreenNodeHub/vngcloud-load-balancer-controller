package ingress_uc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		t.logger.Debug("ingress has default backend, building default pool")
		defaultPool, err = t.buildPool(ctx, t.ingress.Spec.DefaultBackend.Service, targetNodeLabels)
		switch {
		case errors.Is(err, errBackendUnresolvable):
			// leave the default backend out; the rules below are still worth serving
			defaultPool = nil
		case err != nil:
			return nil, nil, err
		default:
			defaultPool.Name = domain.DEFAULT_NAME_DEFAULT_POOL
			appendPool(*defaultPool)
		}
	}

	// build listener http
	isNeedHTTPListener := t.checkNeedHTTPListener(ctx)
	var httpListener *v1alpha1.Listener
	if isNeedHTTPListener {
		httpListener, err = t.buildListeners(ctx, false)
		if err != nil {
			return nil, nil, err
		}
		// defaultPool is nil when the default backend's Service does not exist
		if defaultPool != nil {
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
		if defaultPool != nil {
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
				if errors.Is(err, errBackendUnresolvable) {
					// drop this path only: no pool, and no policy pointing at one
					continue
				}
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

	if options := t.buildAnnotationAutoReorderPolicies(ctx); options != nil && *options {
		if t.buildAutoAddPolicyPosition(ctx, httpListener) != nil {
			return nil, nil, err
		}
		if t.buildAutoAddPolicyPosition(ctx, httpsListener) != nil {
			return nil, nil, err
		}
	}

	if isNeedHTTPListener {
		allListeners = append(allListeners, *httpListener)
	}

	if isValidToBuildHTTPSListener {
		allListeners = append(allListeners, *httpsListener)
	}

	// Sort pools and listeners by name to ensure deterministic ordering
	// This prevents unnecessary reconciliation loops caused by array order changes
	sort.Slice(allPools, func(i, j int) bool {
		return allPools[i].Name < allPools[j].Name
	})
	sort.Slice(allListeners, func(i, j int) bool {
		return allListeners[i].Name < allListeners[j].Name
	})
	// Sort policies within each listener by name
	for i := range allListeners {
		sort.Slice(allListeners[i].Policies, func(a, b int) bool {
			return allListeners[i].Policies[a].Name < allListeners[i].Policies[b].Name
		})
	}

	return allPools, allListeners, nil
}

// from serviceBackend include name and port, build the pool
// errBackendUnresolvable marks a backend that cannot be turned into a pool because of how
// the Ingress refers to it: the Service does not exist, or it exists but does not expose the
// port named. Callers drop just that path instead of failing the whole Ingress - one bad
// backend reference, a typo or a chart whose Service has not been applied yet, must not take
// the other rules down with it.
//
// Every other error stays fatal, so a transient API failure cannot silently remove routes
// from the load balancer.
var errBackendUnresolvable = errors.New("backend cannot be resolved")

// backendPortDescription renders whichever half of ServiceBackendPort the Ingress filled in,
// so the log says what was asked for rather than "0" or "".
func backendPortDescription(port networkingv1.ServiceBackendPort) string {
	if port.Name != "" {
		return strconv.Quote(port.Name)
	}
	return strconv.Itoa(int(port.Number))
}

func (t *defaultModelBuildTask) buildPool(ctx context.Context, service *networkingv1.IngressServiceBackend, targetNodeLabels map[string]string) (*v1alpha1.Pool, error) {
	// find service
	findService, err := t.k8sRepo.GetService(ctx, types.NamespacedName{Namespace: t.ingress.GetNamespace(), Name: service.Name})
	if err != nil {
		if apierrors.IsNotFound(err) {
			t.logger.Warnf("service %s/%s not found, skipping this backend", t.ingress.GetNamespace(), service.Name)
			return nil, fmt.Errorf("%w: %s/%s", errBackendUnresolvable, t.ingress.GetNamespace(), service.Name)
		}
		return nil, fmt.Errorf("failed to get service %s/%s: %w", t.ingress.GetNamespace(), service.Name, err)
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
		// The Service is there but does not serve the port this rule asks for. Nothing here
		// will change until the Ingress or the Service does, and it is one path's problem.
		t.logger.Warnf("service %s/%s does not expose port %s, skipping this backend",
			t.ingress.GetNamespace(), service.Name, backendPortDescription(service.Port))
		return nil, fmt.Errorf("%w: %s/%s has no port %s", errBackendUnresolvable,
			t.ingress.GetNamespace(), service.Name, backendPortDescription(service.Port))
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
			return nil, fmt.Errorf("failed to resolve node port endpoints for service %s/%s: %w", t.ingress.GetNamespace(), service.Name, err)
		}
	} else {
		membersAddr, err = t.endpointResolver.ResolvePodEndpoints(ctx,
			types.NamespacedName{Namespace: t.ingress.GetNamespace(), Name: service.Name}, serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve pod endpoints for service %s/%s: %w", t.ingress.GetNamespace(), service.Name, err)
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

	// Sort pool members by IP:Port to ensure deterministic ordering
	// This prevents unnecessary reconciliation loops caused by array order changes
	sort.Slice(poolMembers, func(i, j int) bool {
		if poolMembers[i].IP != poolMembers[j].IP {
			return poolMembers[i].IP < poolMembers[j].IP
		}
		return poolMembers[i].Port < poolMembers[j].Port
	})

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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixHealthcheckPort, err)
		return nil
	}
	if optionsInt64 <= 0 || optionsInt64 > 65535 {
		t.logger.Debugf("Invalid annotation \"%s\" value %d, must be in range 1-65535.",
			annotations.SuffixHealthcheckPort, optionsInt64)
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
		t.logger.Debugf("Invalid annotation \"%s\" value \"%s\", must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixPoolAlgorithm,
			option,
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
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixIdleTimeoutClient, err)
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
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixIdleTimeoutMember, err)
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
		t.logger.Warnf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixIdleTimeoutConnection, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixHealthyThresholdCount, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixUnhealthyThresholdCount, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixHealthcheckIntervalSeconds, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be an integer: %v",
			annotations.SuffixHealthcheckTimeoutSeconds, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be a boolean: %v",
			annotations.SuffixEnableStickySession, err)
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
		t.logger.Debugf("Invalid annotation \"%s\" value, must be a boolean: %v",
			annotations.SuffixEnableTLSEncryption, err)
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
