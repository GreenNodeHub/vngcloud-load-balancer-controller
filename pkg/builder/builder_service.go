package builder

import (
	"context"
	"strings"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// depend on service provided, build the LoadbalancerBuilder
func NewLoadBalancerBuilderByService(
	ctx context.Context,
	service *corev1.Service,
	annotationParser annotations.Parser,
	client client.Client,
	networkID, subnetID, subnetCIDR string,
	clusterID string,
	nodes []*corev1.Node,
) (LoadbalancerBuilder, error) {
	model := &lbBuilder{
		resourceType:      "service",
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
		LoadBalancerType:           loadbalancerv2.LoadBalancerTypeLayer4,
		packageID:                  consts.DEFAULT_L4_PACKAGE_ID,
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
	if service == nil {
		return model, nil
	}

	model.resourceName = service.Name
	model.resourceNamespace = service.Namespace

	model.parseAnnotation(service.Annotations)

	model.buildService(service, nodes)

	return model, nil
}

func (l *lbBuilder) buildService(pService *corev1.Service, nodes []*corev1.Node) error {
	// Check if the service spec has any port, if not, return error
	ports := pService.Spec.Ports
	if len(ports) <= 0 {
		return errs.ErrorServicePortEmpty
	}

	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.objectToLBName(l.clusterID, pService)
	}

	// Get members address, nodeIP or podIP
	var membersAddr []*MemberAddress
	if l.GetTargetType() == TargetTypeInstance {
		nodesAfterFilter := l.filterByNodeLabel(nodes, l.GetTargetNodeLabels())
		if len(nodesAfterFilter) < 1 {
			l.logger.Warnf("No node available for service %s/%s, pool have no member.", pService.Namespace, pService.Name)
		}
		membersAddr = l.getNodeMembersAddr(nodesAfterFilter)
	} else {
		var err error
		membersAddr, err = l.getEndpointMembersAddr(pService.Namespace, pService.Name)
		if err != nil {
			return err
		}
	}

	l.logger.Debugf("Members address: %v", membersAddr)

	// Build pool and listener
	for _, port := range ports {
		poolName := l.genL4PoolName(port)
		listenerName := l.genL4ListenerName(port)

		// nodePort if target type is instance, targetPort if target type is ip
		targetPort := 0
		if l.TargetType == TargetTypeInstance {
			targetPort = int(port.NodePort)
		} else {
			targetPort = int(port.TargetPort.IntValue())
		}

		// add security group rule
		monitorPort := int(port.NodePort)
		if l.HealthcheckPort != 0 {
			monitorPort = l.HealthcheckPort
			if l.IsAutoCreateSecurityGroup && l.HealthcheckPort != int(port.NodePort) {
				l.addSecgroupRule(monitorPort, string(port.Protocol))

				if strings.EqualFold(string(port.Protocol), "UDP") {
					l.addSecgroupRule(monitorPort, "ICMP")
				}
			}
		}
		if l.IsAutoCreateSecurityGroup {
			l.addSecgroupRule(int(port.NodePort), string(port.Protocol))
			if strings.EqualFold(string(port.Protocol), "UDP") {
				l.addSecgroupRule(int(port.NodePort), "ICMP")
			}
		}

		// build pool members
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

		poolBuilder := l.createPoolBuilder(port, poolName)
		poolBuilder.Members = poolMembers

		listenerBuilder := l.createListenerBuilder(port, listenerName)
		listenerBuilder.ReferPoolName = poolName

		l.AddPoolBuilder(poolBuilder)
		l.AddListenerBuilder(listenerBuilder)
	}

	return nil

}

// createListenerBuilder creates the listener options.
func (l *lbBuilder) createListenerBuilder(pPort corev1.ServicePort, name string) *ListenerBuilderType {
	opt := &ListenerBuilderType{
		IsL4: true,
		commonBuilder: commonBuilder{
			name: name,
		},
		isDeleted:      false,
		policyBuilders: []*policyBuilderType{},
		ReferPoolName:  "", // will be set later
		CreateListenerRequest: loadbalancerv2.CreateListenerRequest{
			DefaultPoolId: PointerOf(""), // will be set later

			ListenerName:                name,
			ListenerProtocol:            VNGHelper.ParseListenerProtocol(pPort),
			ListenerProtocolPort:        int(pPort.Port),
			CertificateAuthorities:      nil,
			ClientCertificate:           nil,
			DefaultCertificateAuthority: nil,
			TimeoutClient:               l.IdleTimeoutClient,
			TimeoutMember:               l.IdleTimeoutMember,
			TimeoutConnection:           l.IdleTimeoutConnection,
			AllowedCidrs:                StringListToString(l.InboundCIDRs),
		},
	}
	return opt
}

// createPoolBuilder creates the pool options.
func (l *lbBuilder) createPoolBuilder(pPort corev1.ServicePort, name string) *poolBuilderType {
	healthMonitor := loadbalancerv2.HealthMonitor{
		HealthyThreshold:    l.HealthyThresholdCount,
		UnhealthyThreshold:  l.UnhealthyThresholdCount,
		Interval:            l.HealthcheckIntervalSeconds,
		Timeout:             l.HealthcheckTimeoutSeconds,
		HealthCheckProtocol: VNGHelper.ParseHealthCheckProtocol(pPort.Protocol, string(l.HealthcheckProtocol)),
	}
	if l.HealthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP ||
		l.HealthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTPs {
		healthMonitor = loadbalancerv2.HealthMonitor{
			HealthyThreshold:    l.HealthyThresholdCount,
			UnhealthyThreshold:  l.UnhealthyThresholdCount,
			Interval:            l.HealthcheckIntervalSeconds,
			Timeout:             l.HealthcheckTimeoutSeconds,
			HealthCheckProtocol: VNGHelper.ParseHealthCheckProtocol(pPort.Protocol, string(l.HealthcheckProtocol)),
			HealthCheckMethod:   PointerOf(l.HealthcheckHttpMethod),
			HealthCheckPath:     PointerOf(l.HealthcheckPath),
			SuccessCode:         PointerOf(l.SuccessCodes),
			HttpVersion:         PointerOf(l.HealthcheckHttpVersion),
			DomainName:          PointerOf(l.HealthcheckHttpDomainName),
		}
	}
	opt := &poolBuilderType{
		IsL4: true,
		commonBuilder: commonBuilder{
			name: name,
		},
		isDeleted:     false,
		PoolProtocol:  VNGHelper.ParsePoolProtocol(l.mappingProtocol(pPort)),
		Stickiness:    false,
		TLSEncryption: false,
		HealthMonitor: &healthMonitor,
		Algorithm:     l.PoolAlgorithm,
		Members:       nil, // will be set later
	}
	for _, name := range l.EnableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == corev1.ProtocolTCP {
			opt.PoolProtocol = loadbalancerv2.PoolProtocolProxy
			break
		}
	}
	return opt
}
