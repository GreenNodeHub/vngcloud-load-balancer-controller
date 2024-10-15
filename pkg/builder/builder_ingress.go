package builder

import (
	"context"
	"fmt"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
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
) (ModelBuilder, error) {
	model := &modelBuilder{
		poolListenerHelper: poolListenerHelper{
			poolBuilders:     make([]*poolBuilderType, 0),
			listenerBuilders: make([]*ListenerBuilderType, 0),
		},

		resourceType:      "ingress",
		resourceName:      "",
		resourceNamespace: "",

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),

		annotationParser: annotationParser,
		context:          ctx,
		client:           client,
		logger:           contexts.NewContext(ctx).Log(),

		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: subnetCIDR,
		clusterID:  clusterID,

		isIgnored: false,

		loadBalancerID:             "",
		loadBalancerName:           "",
		loadBalancerType:           loadbalancerv2.LoadBalancerTypeLayer7,
		packageID:                  consts.DEFAULT_L7_PACKAGE_ID,
		scheme:                     loadbalancerv2.InternetLoadBalancerScheme,
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
		tags:                       map[string]string{},
		targetNodeLabels:           map[string]string{},
		IsAutoCreateSecurityGroup:  false,
		securityGroups:             []string{},
		enableProxyProtocol:        []string{},
		enableAutoscale:            false,
		targetType:                 TargetTypeInstance,
		enableStickySession:        false,
		enableTLSEncryption:        false,
		certificateIDs:             []string{},
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

	model.logger.Debugf("New model listeners: len(%d)", len(model.listenerBuilders))
	model.logger.Debugf("New model pools: (%d)", len(model.poolBuilders))

	return model, nil
}

func (l *modelBuilder) buildIngress(ingress *networkingv1.Ingress, nodes []*corev1.Node) error {
	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.objectToLBName()
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
	httpListener, err := l.buildL7Listener(false)
	if err != nil {
		return err
	}
	if isHaveDefaultBackend {
		httpListener.ReferPoolName = defaultPoolBuilder.GetName()
	}
	l.AddListenerBuilder(httpListener)

	// build listener https
	var httpsListener *ListenerBuilderType
	if len(ingress.Spec.TLS) > 0 {
		if len(l.certificateIDs) < 1 {
			return errs.ErrorMissingCertificates
		}
		httpsListener, err = l.buildL7Listener(true)
		if err != nil {
			return err
		}
		httpsListener.DefaultCertificateAuthority = &l.certificateIDs[0]
		httpsListener.CertificateAuthorities = &l.certificateIDs
		httpsListener.ClientCertificate = PointerOf("")
		if isHaveDefaultBackend {
			httpListener.ReferPoolName = defaultPoolBuilder.GetName()
		}
		l.AddListenerBuilder(httpsListener)
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
			CertificateAuthorities:      nil,
			ClientCertificate:           nil,
			DefaultCertificateAuthority: nil,
		},
	}
	if isHTTPS {
		opt.name = consts.DEFAULT_HTTPS_LISTENER_NAME
		opt.ListenerName = consts.DEFAULT_HTTPS_LISTENER_NAME
		opt.ListenerProtocol = loadbalancerv2.ListenerProtocolHTTPS
		opt.ListenerProtocolPort = 443
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

	poolName := l.genL7PoolName(int(servicePort.Port))

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
	} else {
		membersAddr, err = endpointResolver.ResolvePodEndpoints(l.context,
			types.NamespacedName{Namespace: l.resourceNamespace, Name: service.Name},
			serviceBackendToIntOrString(service.Port), resolveOpts...)
		if err != nil {
			return nil, err
		}
	}

	// build members
	poolMembers := make([]*loadbalancerv2.Member, 0)
	for _, member := range membersAddr {
		// get monitor port
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

func (l *modelBuilder) genL7PoolName(port int) string {
	hash := l.generateHash()
	name := fmt.Sprintf("%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash,
		TrimString(fmt.Sprintf("%s-%s", l.resourceNamespace, l.resourceName), 35),
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
