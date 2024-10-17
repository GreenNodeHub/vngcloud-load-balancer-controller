package builder

import (
	"context"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// depend on service provided, build the ModelBuilder
func NewModelBuilderByService(
	ctx context.Context,
	service *corev1.Service,
	annotationParser annotations.Parser,
	client client.Client,
	networkID, subnetID, subnetCIDR string,
	clusterID string,
	nodes []*corev1.Node,
	cnitype utils.CNIType,
) (ModelBuilder, error) {
	model := &modelBuilder{
		poolListenerHelper: poolListenerHelper{
			poolBuilders:     make([]*poolBuilderType, 0),
			listenerBuilders: make([]*ListenerBuilderType, 0),
		},
		basicInfoHelper: basicInfoHelper{
			loadBalancerID:   "",
			loadBalancerName: "",
			loadBalancerType: loadbalancerv2.LoadBalancerTypeLayer4,
			packageID:        consts.DEFAULT_L4_PACKAGE_ID,
			scheme:           loadbalancerv2.InternetLoadBalancerScheme,
			tags:             map[string]string{},
		},
		resourceType:            "service",
		resourceName:            "",
		resourceNamespace:       "",
		loadBalancerDefaultName: "",

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),

		annotationParser: annotationParser,
		context:          ctx,
		client:           client,
		logger:           contexts.NewContext(ctx).Log(),
		cniType:          cnitype,

		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: subnetCIDR,
		clusterID:  clusterID,

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

		isAutoCreateSecurityGroup: true,
	}
	if service == nil {
		return model, nil
	}

	model.resourceName = service.Name
	model.resourceNamespace = service.Namespace
	model.loadBalancerDefaultName = model.objectToLBName()

	model.parseAnnotation(service.Annotations)

	err := model.buildService(service, nodes)
	if err != nil {
		return nil, err
	}

	return model, nil
}

func (l *modelBuilder) buildService(pService *corev1.Service, _ []*corev1.Node) error {
	// Check if the service spec has any port, if not, return error
	ports := pService.Spec.Ports
	if len(ports) <= 0 {
		return errs.ErrorServicePortEmpty
	}

	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.objectToLBName()
	}

	// Get members address, nodeIP or podIP
	endpointResolver := utils.NewDefaultEndpointResolver(l.context, l.client)
	resolveOpts := []utils.EndpointResolveOption{
		utils.WithNodeSelector(labels.SelectorFromSet(labels.Set(l.GetTargetNodeLabels()))),
	}

	// Build pool and listener
	for _, port := range ports {
		poolName := l.genL4PoolName(port)
		listenerName := l.genL4ListenerName(port)

		// nodePort if target type is instance, targetPort if target type is ip
		var membersAddr []utils.EndpointAddress
		var err error
		if l.targetType == TargetTypeInstance {
			membersAddr, err = endpointResolver.ResolveNodePortEndpoints(l.context,
				namespacedName(pService), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return err
			}

			// if cniType is cilium nr, add pod port to secgroup
			if l.cniType == utils.CiliumNativeRouting {
				podPorts, err := endpointResolver.GetListTargetPort(l.context, namespacedName(pService), intstr.FromInt(int(port.Port)))
				if err != nil {
					return err
				}
				for _, podPort := range podPorts {
					l.addDefaultSecgroupRules(podPort, coreProtocolToSecgroupProtocol(port.Protocol))
				}
			}
		} else {
			membersAddr, err = endpointResolver.ResolvePodEndpoints(l.context,
				namespacedName(pService), intstr.FromInt(int(port.Port)), resolveOpts...)
			if err != nil {
				return err
			}
		}

		// if healthcheckPort is set, add to secgroup
		if l.healthcheckPort != 0 {
			l.addDefaultSecgroupRules(l.healthcheckPort, coreProtocolToSecgroupProtocol(port.Protocol))
		}

		// build pool members
		poolMembers := make([]*loadbalancerv2.Member, 0)
		for _, member := range membersAddr {
			// L4 support TCP (TCP, HTTP, HTTPS), UDP (PING-UDP), PROXY (TCP, HTTP, HTTPS)
			l.addDefaultSecgroupRules(member.Port, coreProtocolToSecgroupProtocol(port.Protocol))

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
func (l *modelBuilder) createListenerBuilder(pPort corev1.ServicePort, name string) *ListenerBuilderType {
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
func (l *modelBuilder) createPoolBuilder(pPort corev1.ServicePort, name string) *poolBuilderType {
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
