package builder

import (
	"context"
	"fmt"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// depend on ingress provided, build the ModelBuilder
func NewModelBuilderByIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	annotationParser annotations.Parser,
	client client.Client,
	networkID, subnetID, subnetCIDR string,
	clusterID string,
	nodes []*corev1.Node,
	cniType utils.CNIType,
	packageID string,
) (ModelBuilder, error) {
	model := &modelBuilder{
		poolListenerHelper: poolListenerHelper{
			poolBuilders:     make([]*poolBuilderType, 0),
			listenerBuilders: make([]*ListenerBuilderType, 0),
		},
		basicInfoHelper: basicInfoHelper{
			loadBalancerID:   "",
			loadBalancerName: "",
			loadBalancerType: loadbalancerv2.LoadBalancerTypeLayer7,
			packageID:        packageID,
			scheme:           loadbalancerv2.InternetLoadBalancerScheme,
			tags:             map[string]string{},
		},
		nameHelper: nameHelper{
			resourceType:      "ingress",
			resourceName:      "",
			resourceNamespace: "",
			clusterID:         clusterID,
		},

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),

		annotationParser: annotationParser,
		context:          ctx,
		client:           client,
		logger:           contexts.NewContext(ctx).Log(),
		cniType:          cniType,

		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: subnetCIDR,

		isIgnored: false,

		idleTimeoutClient:          50,
		idleTimeoutMember:          50,
		idleTimeoutConnection:      5,
		inboundCIDRs:               []string{"0.0.0.0/0"},
		healthcheckProtocol:        loadbalancerv2.HealthCheckProtocolTCP,
		healthcheckHttpMethod:      loadbalancerv2.HealthCheckMethodGET,
		healthcheckPath:            "/",
		successCodes:               "200",
		healthcheckHttpVersion:     loadbalancerv2.HealthCheckHttpVersionHttp1,
		healthcheckHttpDomainName:  "",
		poolAlgorithm:              loadbalancerv2.PoolAlgorithmRoundRobin,
		healthyThresholdCount:      3,
		unhealthyThresholdCount:    3,
		healthcheckTimeoutSeconds:  5,
		healthcheckIntervalSeconds: 30,
		healthcheckPort:            0,
		targetNodeLabels:           map[string]string{},
		securityGroups:             []string{},
		enableProxyProtocol:        []string{},
		enableAutoscale:            false,
		targetType:                 TargetTypeInstance,
		enableStickySession:        false,
		enableTLSEncryption:        false,
		certificateIDs:             []string{},
		headers:                    []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Port"},

		isAutoCreateSecurityGroup: true,
	}
	if ingress == nil {
		return model, nil
	}

	model.resourceName = ingress.Name
	model.resourceNamespace = ingress.Namespace

	model.parseAnnotation(ingress.Annotations)

	err := model.buildIngress(ingress, nodes)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (l *modelBuilder) buildIngress(ingress *networkingv1.Ingress, nodes []*corev1.Node) error {
	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.GetLoadBalancerDefaultName()
	}

	// build default backend pool
	isHaveDefaultBackend := ingress.Spec.DefaultBackend != nil
	var defaultPoolBuilder *poolBuilderType
	if isHaveDefaultBackend {
		l.logger.Debugf("Ingress %s/%s has default backend, building default pool.", l.resourceNamespace, l.resourceName)
		var err error
		defaultPoolBuilder, err = l.buildIngressPool(ingress.Spec.DefaultBackend.Service, nodes)
		if err != nil {
			return err
		}
		defaultPoolBuilder.SetName(consts.DEFAULT_NAME_DEFAULT_POOL)
		l.AddPoolBuilder(defaultPoolBuilder)
	}

	// build listener http
	var err error
	var httpListener *ListenerBuilderType
	if l.checkNeedHTTPListener(ingress) {
		httpListener, err = l.buildL7Listener(false)
		if err != nil {
			return err
		}
		if isHaveDefaultBackend {
			httpListener.ReferPoolName = defaultPoolBuilder.GetName()
		}
		l.AddListenerBuilder(httpListener)
	}

	// build listener https
	var httpsListener *ListenerBuilderType
	if len(l.certificateIDs) > 0 {
		httpsListener, err = l.buildL7Listener(true)
		if err != nil {
			return err
		}
		if isHaveDefaultBackend {
			httpsListener.ReferPoolName = defaultPoolBuilder.GetName()
		}
		l.AddListenerBuilder(httpsListener)
	} else if len(ingress.Spec.TLS) > 0 {
		return errs.ErrorMissingCertificates
	}

	// which host using tls
	hostUsingTLS := make(map[string]bool)
	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			hostUsingTLS[host] = true
		}
	}

	// build policy
	for ruleIndex, rule := range ingress.Spec.Rules {
		_, isHttpsListener := hostUsingTLS[rule.Host]

		for pathIndex, path := range rule.HTTP.Paths {
			// build pool
			poolBuilder, err := l.buildIngressPool(path.Backend.Service, nodes)
			if err != nil {
				return err
			}
			l.AddPoolBuilder(poolBuilder)

			// build policy
			policyName := l.genL7PolicyName(isHttpsListener, ruleIndex, pathIndex)
			policyBuilder, err := l.buildPolicyByPath(rule.Host, policyName, &path)
			if err != nil {
				return err
			}
			policyBuilder.ReferPoolName = poolBuilder.GetName()
			if isHttpsListener {
				httpsListener.policyBuilders = append(httpsListener.policyBuilders, policyBuilder)
			} else {
				httpListener.policyBuilders = append(httpListener.policyBuilders, policyBuilder)
			}
		}
	}

	return nil
}

// if have default backend,
// if have host not in tls,
// if have host = ""
func (l *modelBuilder) checkNeedHTTPListener(ingress *networkingv1.Ingress) bool {
	if ingress.Spec.DefaultBackend != nil {
		l.logger.Debugf("Need HTTP listener: has default backend")
		return true
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			l.logger.Debugf("Need HTTP listener: has rule with empty host")
			return true
		}
	}
	tlsHosts := make(map[string]bool)
	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			tlsHosts[host] = true
		}
	}
	for _, rule := range ingress.Spec.Rules {
		if _, ok := tlsHosts[rule.Host]; !ok {
			l.logger.Debugf("Need HTTP listener: has rule with host not in tls")
			return true
		}
	}
	return false
}

// buildL7Listener
func (l *modelBuilder) buildL7Listener(isHTTPS bool) (*ListenerBuilderType, error) {
	opt := &ListenerBuilderType{
		isDeleted:      false,
		ReferPoolName:  "",
		IsL4:           false,
		policyBuilders: make([]*policyBuilderType, 0),
		commonBuilder: commonBuilder{
			name: consts.DEFAULT_HTTP_LISTENER_NAME,
		},
		CreateListenerRequest: loadbalancerv2.CreateListenerRequest{
			AllowedCidrs:                StringListToString(l.inboundCIDRs),
			ListenerName:                consts.DEFAULT_HTTP_LISTENER_NAME,
			ListenerProtocol:            loadbalancerv2.ListenerProtocolHTTP,
			ListenerProtocolPort:        80,
			TimeoutClient:               l.idleTimeoutClient,
			TimeoutConnection:           l.idleTimeoutConnection,
			TimeoutMember:               l.idleTimeoutMember,
			DefaultPoolId:               PointerOf(""),
			CertificateAuthorities:      PointerOf([]string{}),
			ClientCertificate:           nil,
			DefaultCertificateAuthority: nil,
			Headers:                     &l.headers,
		},
	}
	if isHTTPS {
		opt.name = consts.DEFAULT_HTTPS_LISTENER_NAME
		opt.ListenerName = consts.DEFAULT_HTTPS_LISTENER_NAME
		opt.ListenerProtocol = loadbalancerv2.ListenerProtocolHTTPS
		opt.ListenerProtocolPort = 443
		opt.DefaultCertificateAuthority = nil
		opt.CertificateAuthorities = PointerOf([]string{})

		if len(l.certificateIDs) > 0 {
			opt.DefaultCertificateAuthority = &l.certificateIDs[0]
		}
		if len(l.certificateIDs) > 1 {
			opt.CertificateAuthorities = PointerOf(l.certificateIDs[1:])
		}
	}
	return opt, nil
}

// from serviceBackend include name and port, build the pool
func (l *modelBuilder) buildIngressPool(service *networkingv1.IngressServiceBackend, _ []*corev1.Node) (*poolBuilderType, error) {
	// find service
	findService := &corev1.Service{}
	err := l.client.Get(l.context, types.NamespacedName{Namespace: l.resourceNamespace, Name: service.Name}, findService)
	if err != nil {
		l.logger.Errorf("Failed to get service %s/%s: %v", l.resourceNamespace, service.Name, err)
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
		return nil, errs.ErrorServicePortNotFound
	}

	poolName := l.genL7PoolName(service.Name, int(servicePort.Port))

	// Get members address, nodeIP or podIP
	endpointResolver := utils.NewDefaultEndpointResolver(l.context, l.client)
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(l.GetTargetNodeLabels()))),
	}

	// nodePort if target type is instance, targetPort if target type is ip
	var membersAddr []utils.EndpointAddress
	if l.targetType == TargetTypeInstance {
		membersAddr, err = endpointResolver.ResolveNodePortEndpoints(l.context,
			types.NamespacedName{Namespace: l.resourceNamespace, Name: service.Name},
			serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			return nil, err
		}

		// if cniType is cilium nr, add pod port to secgroup
		if l.cniType == utils.CiliumNativeRouting {
			podPorts, err := endpointResolver.GetListTargetPort(l.context, types.NamespacedName{Namespace: l.resourceNamespace, Name: service.Name},
				serviceBackendToIntOrString(service.Port))
			if err != nil {
				return nil, err
			}
			for _, podPort := range podPorts {
				l.addDefaultSecgroupRules(podPort, networkv2.SecgroupRuleProtocolTCP)
			}
		}
	} else {
		membersAddr, err = endpointResolver.ResolvePodEndpoints(l.context,
			types.NamespacedName{Namespace: l.resourceNamespace, Name: service.Name},
			serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			return nil, err
		}
	}

	// if healthcheckPort is set, add to secgroup
	if l.healthcheckPort != 0 {
		l.addDefaultSecgroupRules(l.healthcheckPort, networkv2.SecgroupRuleProtocolTCP)
	}

	// build members
	poolMembers := make([]*loadbalancerv2.Member, 0)
	for _, member := range membersAddr {
		// L7 pool only support TCP and HTTP healthcheck
		l.addDefaultSecgroupRules(member.Port, networkv2.SecgroupRuleProtocolTCP)

		monitorPort := member.Port
		if l.healthcheckPort != 0 {
			monitorPort = l.healthcheckPort
		}

		poolMembers = append(poolMembers, &loadbalancerv2.Member{
			IpAddress:   member.IP,
			Port:        member.Port,
			Backup:      false,
			Weight:      1,
			MonitorPort: monitorPort,
			Name:        member.Name,
		})
	}

	// build healthcheck
	healthMonitor := loadbalancerv2.HealthMonitor{
		HealthyThreshold:    l.healthyThresholdCount,
		UnhealthyThreshold:  l.unhealthyThresholdCount,
		Interval:            l.healthcheckIntervalSeconds,
		Timeout:             l.healthcheckTimeoutSeconds,
		HealthCheckProtocol: l.healthcheckProtocol,
	}
	if l.healthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {
		healthMonitor = loadbalancerv2.HealthMonitor{
			HealthyThreshold:    l.healthyThresholdCount,
			UnhealthyThreshold:  l.unhealthyThresholdCount,
			Interval:            l.healthcheckIntervalSeconds,
			Timeout:             l.healthcheckTimeoutSeconds,
			HealthCheckProtocol: l.healthcheckProtocol,
			HealthCheckMethod:   PointerOf(l.healthcheckHttpMethod),
			HealthCheckPath:     PointerOf(l.healthcheckPath),
			SuccessCode:         PointerOf(l.successCodes),
			HttpVersion:         PointerOf(l.healthcheckHttpVersion),
			DomainName:          PointerOf(l.healthcheckHttpDomainName),
		}
	}

	// build pool
	opt := &poolBuilderType{
		IsL4: false,
		commonBuilder: commonBuilder{
			name: poolName,
		},
		PoolProtocol:  loadbalancerv2.PoolProtocolHTTP,
		Stickiness:    l.enableStickySession,
		TLSEncryption: l.enableTLSEncryption,
		HealthMonitor: &healthMonitor,
		Algorithm:     l.poolAlgorithm,
		Members:       poolMembers,
	}
	return opt, nil
}

func (l *modelBuilder) genL7PoolName(serviceName string, port int) string {
	hash := l.generateHash()
	name := fmt.Sprintf("%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash,
		TrimString(fmt.Sprintf("%s-%s", l.resourceNamespace, serviceName), 35),
		port)
	return l.validateName(name)
}

func (l *modelBuilder) genL7PolicyName(mode bool, ruleIndex, pathIndex int) string {
	hash := l.generateHash()
	name := fmt.Sprintf("%s_%s_%t_r%d_p%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash, mode, ruleIndex, pathIndex)
	return l.validateName(name)
}

func (l *modelBuilder) buildPolicyByPath(host, policyName string, path *networkingv1.HTTPIngressPath) (*policyBuilderType, error) {
	// compare type
	var compareType loadbalancerv2.PolicyCompareType
	switch *path.PathType {
	case networkingv1.PathTypeExact:
		compareType = loadbalancerv2.PolicyCompareTypeEQUALS
	case networkingv1.PathTypePrefix:
		compareType = loadbalancerv2.PolicyCompareTypeSTARTSWITH
	// case networkingv1.PathTypeImplementationSpecific:
	// 	compareType = loadbalancerv2.PolicyCompareTypeRegex
	default:
		compareType = loadbalancerv2.PolicyCompareTypeEQUALS
	}

	// path rule
	l7Rules := []loadbalancerv2.L7RuleRequest{
		{
			CompareType: compareType,
			RuleType:    loadbalancerv2.PolicyRuleTypePATH,
			RuleValue:   path.Path,
		},
	}

	if host != "" {
		l7Rules = append(l7Rules, loadbalancerv2.L7RuleRequest{
			CompareType: loadbalancerv2.PolicyCompareTypeEQUALS,
			RuleType:    loadbalancerv2.PolicyRuleTypeHOSTNAME,
			RuleValue:   host,
		})
	}

	// build policy
	return &policyBuilderType{
		commonBuilder: commonBuilder{
			name: policyName,
		},
		Action: loadbalancerv2.PolicyActionREDIRECTTOPOOL,
		Rules:  l7Rules,

		isDeleted:     false,
		ReferPoolName: "", // will be set later
	}, nil
}
