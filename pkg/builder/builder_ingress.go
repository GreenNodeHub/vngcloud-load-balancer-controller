package builder

import (
	"context"
	"fmt"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// depend on ingress provided, build the LoadbalancerBuilder
func NewLoadBalancerBuilderByIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	annotationParser annotations.Parser,
	client client.Client,
	networkID, subnetID, subnetCIDR string,
	clusterID string,
	nodes []*corev1.Node,
) (LoadbalancerBuilder, error) {
	model := &lbBuilder{
		resourceType:      "ingress",
		resourceName:      "",
		resourceNamespace: "",

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),
		poolBuilders:         make([]*poolBuilderType, 0),

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
		LoadBalancerType:           loadbalancerv2.LoadBalancerTypeLayer7,
		packageID:                  consts.DEFAULT_L7_PACKAGE_ID,
		Scheme:                     loadbalancerv2.InternetLoadBalancerScheme,
		IdleTimeoutClient:          50,
		IdleTimeoutMember:          50,
		IdleTimeoutConnection:      5,
		InboundCIDRs:               []string{"0.0.0.0/0"},
		HealthcheckProtocol:        loadbalancerv2.HealthCheckProtocolTCP,
		HealthcheckHttpMethod:      loadbalancerv2.HealthCheckMethodGET,
		HealthcheckPath:            "/",
		SuccessCodes:               "200",
		HealthcheckHttpVersion:     loadbalancerv2.HealthCheckHttpVersionHttp1,
		HealthcheckHttpDomainName:  "",
		PoolAlgorithm:              loadbalancerv2.PoolAlgorithmRoundRobin,
		HealthyThresholdCount:      3,
		UnhealthyThresholdCount:    3,
		HealthcheckTimeoutSeconds:  5,
		HealthcheckIntervalSeconds: 30,
		HealthcheckPort:            0,
		Tags:                       map[string]string{},
		TargetNodeLabels:           map[string]string{},
		IsAutoCreateSecurityGroup:  false,
		SecurityGroups:             []string{},
		EnableProxyProtocol:        []string{},
		EnableAutoscale:            false,
	}
	if ingress == nil {
		return model, nil
	}

	model.resourceName = ingress.Name
	model.resourceNamespace = ingress.Namespace

	model.parseAnnotation(ingress.Annotations)

	model.buildIngress(ingress, nodes)

	return model, nil
}

func (l *lbBuilder) buildIngress(ingress *networkingv1.Ingress, nodes []*corev1.Node) error {
	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.objectToLBName(l.clusterID, ingress)
	}

	// build default backend pool
	isHaveDefaultBackend := ingress.Spec.DefaultBackend != nil
	var defaultPoolBuilder *poolBuilderType
	if isHaveDefaultBackend {
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
func (l *lbBuilder) buildL7Listener(isHTTPS bool) (*ListenerBuilderType, error) {
	opt := &ListenerBuilderType{
		isDeleted:      false,
		ReferPoolName:  "",
		IsL4:           false,
		policyBuilders: make([]*policyBuilderType, 0),
		commonBuilder: commonBuilder{
			name: consts.DEFAULT_HTTP_LISTENER_NAME,
		},
		CreateListenerRequest: loadbalancerv2.CreateListenerRequest{
			AllowedCidrs:                StringListToString(l.InboundCIDRs),
			ListenerName:                consts.DEFAULT_HTTP_LISTENER_NAME,
			ListenerProtocol:            loadbalancerv2.ListenerProtocolHTTP,
			ListenerProtocolPort:        80,
			TimeoutClient:               l.IdleTimeoutClient,
			TimeoutConnection:           l.IdleTimeoutConnection,
			TimeoutMember:               l.IdleTimeoutMember,
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
func (l *lbBuilder) buildIngressPool(service *networkingv1.IngressServiceBackend, nodes []*corev1.Node) (*poolBuilderType, error) {
	poolName := l.genL7PoolName(int(service.Port.Number))

	// get nodePort and targetPort
	serviceTargetPort, serviceNodePort, err := l.getServicePort(service)
	if err != nil {
		return nil, err
	}

	// nodePort if target type is instance, targetPort if target type is ip
	targetPort := 0
	if l.TargetType == TargetTypeInstance {
		targetPort = serviceNodePort
	} else {
		targetPort = serviceTargetPort
	}

	// get monitor port
	monitorPort := targetPort
	if l.HealthcheckPort != 0 {
		monitorPort = l.HealthcheckPort
	}

	// Get members address, nodeIP or podIP
	var membersAddr []*MemberAddress
	if l.GetTargetType() == TargetTypeInstance {
		nodesAfterFilter := l.filterByNodeLabel(nodes, l.GetTargetNodeLabels())
		if len(nodesAfterFilter) < 1 {
			l.logger.Warnf("No node available for service %s/%s, pool have no member.", l.resourceNamespace, l.resourceName)
		}
		membersAddr = l.getNodeMembersAddr(nodesAfterFilter)
	} else {
		var err error
		membersAddr, err = l.getEndpointMembersAddr(l.resourceName, service.Name)
		if err != nil {
			return nil, err
		}
	}

	// build members
	poolMembers := make([]*loadbalancerv2.Member, 0)
	for _, member := range membersAddr {
		poolMembers = append(poolMembers, &loadbalancerv2.Member{
			IpAddress:   member.IP,
			Port:        targetPort,
			Backup:      false,
			Weight:      1,
			MonitorPort: monitorPort,
			Name:        member.Name,
		})
	}

	// build healthcheck
	healthMonitor := loadbalancerv2.HealthMonitor{
		HealthyThreshold:    l.HealthyThresholdCount,
		UnhealthyThreshold:  l.UnhealthyThresholdCount,
		Interval:            l.HealthcheckIntervalSeconds,
		Timeout:             l.HealthcheckTimeoutSeconds,
		HealthCheckProtocol: l.HealthcheckProtocol,
	}
	if l.HealthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {
		healthMonitor = loadbalancerv2.HealthMonitor{
			HealthyThreshold:    l.HealthyThresholdCount,
			UnhealthyThreshold:  l.UnhealthyThresholdCount,
			Interval:            l.HealthcheckIntervalSeconds,
			Timeout:             l.HealthcheckTimeoutSeconds,
			HealthCheckProtocol: l.HealthcheckProtocol,
			HealthCheckMethod:   PointerOf(l.HealthcheckHttpMethod),
			HealthCheckPath:     PointerOf(l.HealthcheckPath),
			SuccessCode:         PointerOf(l.SuccessCodes),
			HttpVersion:         PointerOf(l.HealthcheckHttpVersion),
			DomainName:          PointerOf(l.HealthcheckHttpDomainName),
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
		Algorithm:     l.PoolAlgorithm,
		Members:       poolMembers,
	}
	return opt, nil
}

func (l *lbBuilder) genL7PoolName(port int) string {
	hash := l.generateHash()
	name := fmt.Sprintf("%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash,
		TrimString(fmt.Sprintf("%s-%s", l.resourceNamespace, l.resourceName), 35),
		port)
	return l.validateName(name)
}

func (l *lbBuilder) genL7PolicyName(mode bool, ruleIndex, pathIndex int) string {
	hash := l.generateHash()
	name := fmt.Sprintf("%s_%s_%t_r%d_p%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash, mode, ruleIndex, pathIndex)
	return l.validateName(name)
}

func (l *lbBuilder) getServicePort(serviceBackend *networkingv1.IngressServiceBackend) (nodePort, targetPort int, err error) {
	if serviceBackend.Port.Name != "" {
		return 0, 0, errs.ErrorServicePortNameEmpty
	}
	var portInfo intstr.IntOrString
	if serviceBackend.Port.Name != "" {
		portInfo.Type = intstr.String
		portInfo.StrVal = serviceBackend.Port.Name
	} else {
		portInfo.Type = intstr.Int
		portInfo.IntVal = serviceBackend.Port.Number
	}

	// get service
	obj := &corev1.Service{}
	err = l.client.Get(l.context, client.ObjectKey{Name: serviceBackend.Name, Namespace: l.resourceNamespace}, obj)
	if err != nil {
		return 0, 0, err
	}

	targetPort, nodePort = 0, 0
	ports := obj.Spec.Ports
	for _, p := range ports {
		if portInfo.Type == intstr.Int && int(p.Port) == portInfo.IntValue() {
			nodePort = int(p.NodePort)
			targetPort = int(p.TargetPort.IntValue())
			break
		}
		if portInfo.Type == intstr.String && p.Name == portInfo.StrVal {
			nodePort = int(p.NodePort)
			targetPort = int(p.TargetPort.IntValue())
			break
		}
	}

	if nodePort == 0 || targetPort == 0 {
		return 0, 0, errs.ErrorServicePortNotFound
	}

	return targetPort, nodePort, nil
}

func (l *lbBuilder) buildPolicyByPath(host, policyName string, path *networkingv1.HTTPIngressPath) (*policyBuilderType, error) {
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
