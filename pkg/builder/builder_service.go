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
		loadBalancerType:           loadbalancerv2.LoadBalancerTypeLayer4,
		packageID:                  consts.DEFAULT_L4_PACKAGE_ID,
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
		if l.targetType == TargetTypeInstance {
			targetPort = int(port.NodePort)
		} else {
			targetPort = int(port.TargetPort.IntValue())
		}

		// add security group rule
		monitorPort := int(port.NodePort)
		if l.healthcheckPort != 0 {
			monitorPort = l.healthcheckPort
			if l.IsAutoCreateSecurityGroup && l.healthcheckPort != int(port.NodePort) {
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
			TimeoutClient:               l.idleTimeoutClient,
			TimeoutMember:               l.idleTimeoutMember,
			TimeoutConnection:           l.idleTimeoutConnection,
			AllowedCidrs:                StringListToString(l.inboundCIDRs),
		},
	}
	return opt
}

// createPoolBuilder creates the pool options.
func (l *lbBuilder) createPoolBuilder(pPort corev1.ServicePort, name string) *poolBuilderType {
	healthMonitor := loadbalancerv2.HealthMonitor{
		HealthyThreshold:    l.healthyThresholdCount,
		UnhealthyThreshold:  l.unhealthyThresholdCount,
		Interval:            l.healthcheckIntervalSeconds,
		Timeout:             l.healthcheckTimeoutSeconds,
		HealthCheckProtocol: VNGHelper.ParseHealthCheckProtocol(pPort.Protocol, string(l.healthcheckProtocol)),
	}
	if l.healthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP ||
		l.healthcheckProtocol == loadbalancerv2.HealthCheckProtocolHTTPs {
		healthMonitor = loadbalancerv2.HealthMonitor{
			HealthyThreshold:    l.healthyThresholdCount,
			UnhealthyThreshold:  l.unhealthyThresholdCount,
			Interval:            l.healthcheckIntervalSeconds,
			Timeout:             l.healthcheckTimeoutSeconds,
			HealthCheckProtocol: VNGHelper.ParseHealthCheckProtocol(pPort.Protocol, string(l.healthcheckProtocol)),
			HealthCheckMethod:   PointerOf(l.healthcheckHttpMethod),
			HealthCheckPath:     PointerOf(l.healthcheckPath),
			SuccessCode:         PointerOf(l.successCodes),
			HttpVersion:         PointerOf(l.healthcheckHttpVersion),
			DomainName:          PointerOf(l.healthcheckHttpDomainName),
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
		Algorithm:     l.poolAlgorithm,
		Members:       nil, // will be set later
	}
	for _, name := range l.enableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == corev1.ProtocolTCP {
			opt.PoolProtocol = loadbalancerv2.PoolProtocolProxy
			break
		}
	}
	return opt
}
