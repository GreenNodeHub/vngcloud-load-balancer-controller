package builder

import (
	"context"
	"fmt"
	"strings"

	"github.com/anngdinh/operator-helper/contexts"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
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
	networkID, subnetID, networkCIDR string,
	clusterID string,
	region string,
	fleetID string,
	nodes []*corev1.Node,
	cnitype utils.CNIType,
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
			loadBalancerType: global.GlobalLoadBalancerTypeLayer4,
			tags:             map[string]string{},
			packageID:        packageID,
		},
		nameHelper: nameHelper{
			resourceType:      "vglb",
			resourceName:      "",
			resourceNamespace: "",
			clusterID:         clusterID,
			fleetID:           fleetID,
		},

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),

		annotationParser: annotationParser,
		context:          ctx,
		client:           client,
		logger:           contexts.NewContext(ctx).Log(),
		cniType:          cnitype,

		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: networkCIDR,
		region:     region,

		isIgnored: false,

		configClusterID:            "",
		idleTimeoutClient:          50,
		idleTimeoutMember:          50,
		idleTimeoutConnection:      5,
		inboundCIDRs:               []string{"0.0.0.0/0"},
		healthcheckProtocol:        global.GlobalPoolHealthCheckProtocolTCP,
		healthcheckHttpMethod:      global.GlobalPoolHealthCheckMethodGET,
		healthcheckPath:            "/",
		successCodes:               "200",
		healthcheckHttpVersion:     global.GlobalPoolHealthCheckHttpVersionHttp1,
		healthcheckHttpDomainName:  "",
		poolAlgorithm:              global.GlobalPoolAlgorithmRoundRobin,
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

		isAutoCreateSecurityGroup: true,
		isPOC:                     false,
		headers:                   headerConfig{},
	}
	if service == nil {
		return model, nil
	}

	model.resourceName = service.Name
	model.resourceNamespace = service.Namespace

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
		return errs.NewNoNeedRequeue("service has no port")
	}

	// check if the service has a name or not, if not, generate a name
	if l.loadBalancerName == "" {
		l.loadBalancerName = l.GetLoadBalancerDefaultName()
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
					l.addDefaultSecgroupRules(podPort, l.coreProtocolToSecgroupProtocol(port.Protocol))
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
			l.addDefaultSecgroupRules(l.healthcheckPort, l.coreProtocolToSecgroupProtocol(port.Protocol))
		}

		// build pool members
		poolMembers := make([]global.IGlobalMemberRequest, 0)
		for _, member := range membersAddr {
			// L4 support TCP (TCP, HTTP, HTTPS), UDP (PING-UDP), PROXY (TCP, HTTP, HTTPS)
			l.addDefaultSecgroupRules(member.Port, l.coreProtocolToSecgroupProtocol(port.Protocol))

			monitorPort := member.Port
			if l.healthcheckPort != 0 {
				monitorPort = l.healthcheckPort
			}

			poolMembers = append(poolMembers, &global.GlobalMemberRequest{
				// IpAddress:   member.IP,
				// Backup:      false,
				Address:     member.IP,
				BackupRole:  false,
				Description: member.Name,
				SubnetID:    l.subnetID,
				Port:        member.Port,
				Weight:      1,
				MonitorPort: monitorPort,
				Name:        l.clusterID,
			})
		}

		poolBuilder := l.createPoolBuilder(port, poolName)
		poolBuilder.GlobalPoolMembers = []*poolMemberBuilderType{
			{
				GlobalPoolMemberRequest: global.GlobalPoolMemberRequest{
					Name:        fmt.Sprintf("%s-%s", l.region, l.networkID),
					Description: "",
					Region:      l.region,
					TrafficDial: 100,
					VPCID:       l.networkID,
					Members:     poolMembers,
					Type:        global.GlobalPoolMemberTypePrivate,
				},
				id: "",
			},
		}

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
		commonBuilder: commonBuilder{
			name: name,
		},
		isDeleted:      false,
		policyBuilders: []*policyBuilderType{},
		ReferPoolName:  "", // will be set later
		CreateGlobalListenerRequest: global.CreateGlobalListenerRequest{
			Description:       "",
			Name:              name,
			Port:              int(pPort.Port),
			Protocol:          global.GlobalListenerProtocolTCP,
			GlobalPoolId:      "",
			TimeoutClient:     l.idleTimeoutClient,
			TimeoutMember:     l.idleTimeoutMember,
			TimeoutConnection: l.idleTimeoutConnection,
			AllowedCidrs:      StringListToString(l.inboundCIDRs),
			Headers:           nil,
		},
	}
	return opt
}

// createPoolBuilder creates the pool options.
func (l *modelBuilder) createPoolBuilder(pPort corev1.ServicePort, name string) *poolBuilderType {
	healthMonitor := global.GlobalHealthMonitorRequest{
		HealthyThreshold:    l.healthyThresholdCount,
		UnhealthyThreshold:  l.unhealthyThresholdCount,
		Interval:            l.healthcheckIntervalSeconds,
		Timeout:             l.healthcheckTimeoutSeconds,
		HealthCheckProtocol: l.parseHealthCheckProtocol(pPort.Protocol, string(l.healthcheckProtocol)),
		HttpMethod:          nil,
		HttpVersion:         nil,
		Path:                nil,
		DomainName:          nil,
		SuccessCode:         nil,
	}
	if l.healthcheckProtocol == global.GlobalPoolHealthCheckProtocolHTTP ||
		l.healthcheckProtocol == global.GlobalPoolHealthCheckProtocolHTTPs {
		healthMonitor = global.GlobalHealthMonitorRequest{
			HealthyThreshold:    l.healthyThresholdCount,
			UnhealthyThreshold:  l.unhealthyThresholdCount,
			Interval:            l.healthcheckIntervalSeconds,
			Timeout:             l.healthcheckTimeoutSeconds,
			HealthCheckProtocol: l.parseHealthCheckProtocol(pPort.Protocol, string(l.healthcheckProtocol)),
			HttpMethod:          PointerOf(l.healthcheckHttpMethod),
			Path:                PointerOf(l.healthcheckPath),
			SuccessCode:         PointerOf(l.successCodes),
			HttpVersion:         PointerOf(l.healthcheckHttpVersion),
			DomainName:          PointerOf(l.healthcheckHttpDomainName),
		}
	}
	opt := &poolBuilderType{
		commonBuilder: commonBuilder{
			name: name,
		},
		Algorithm:         global.GlobalPoolAlgorithmRoundRobin,
		isDeleted:         false,
		Description:       "",
		Name:              name,
		Protocol:          global.GlobalPoolProtocolTCP,
		Stickiness:        nil,
		TLSEncryption:     nil,
		HealthMonitor:     &healthMonitor,
		GlobalPoolMembers: nil, // will be set later
	}
	for _, name := range l.enableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == corev1.ProtocolTCP {
			opt.Protocol = global.GlobalPoolProtocolProxy
			break
		}
	}
	return opt
}

// genL4ListenerName generates the name of the listener.
func (l *modelBuilder) genL4ListenerName(pPort corev1.ServicePort) string {
	hash := l.GenerateHash()
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(l.fleetID, 10),
		TrimString(l.resourceNamespace, 9),
		TrimString(l.resourceName, 9),
		hash,
		TrimString(string(pPort.Protocol), 3),
		pPort.Port)
	return l.validateName(name)
}

// genL4PoolName generates the name of the pool.
func (l *modelBuilder) genL4PoolName(pPort corev1.ServicePort) string {
	realProtocol := l.mappingProtocol(pPort)

	hash := l.GenerateHash()
	name := fmt.Sprintf("%s_%s_%s_%s_%s_%s_%d",
		consts.DEFAULT_LB_PREFIX_NAME,
		TrimString(l.fleetID, 10),
		TrimString(l.resourceNamespace, 9),
		TrimString(l.resourceName, 9),
		hash,
		TrimString(realProtocol, 3),
		pPort.Port)
	return l.validateName(name)
}

// mappingProtocol maps the protocol TCP to the protocol PROXY if have configured.
func (l *modelBuilder) mappingProtocol(pPort corev1.ServicePort) string {
	for _, name := range l.enableProxyProtocol {
		if (name == "*" || name == pPort.Name) && pPort.Protocol == corev1.ProtocolTCP {
			return string(global.GlobalPoolProtocolProxy)
		}
	}
	return string(pPort.Protocol)
}

// ParseListenerProtocol parse listener protocol to listener protocol
func (l *modelBuilder) parseListenerProtocol(pPort corev1.ServicePort) loadbalancerv2.ListenerProtocol {
	opt := strings.TrimSpace(strings.ToUpper(string(pPort.Protocol)))
	switch opt {
	case string(loadbalancerv2.ListenerProtocolUDP):
		return loadbalancerv2.ListenerProtocolUDP
	}

	return loadbalancerv2.ListenerProtocolTCP
}

// ParseMonitorProtocol parse monitor protocol to health check protocol
func (l *modelBuilder) parseHealthCheckProtocol(pPoolProtocol corev1.Protocol, pMonitorProtocol string) global.GlobalPoolHealthCheckProtocol {
	// switch pPoolProtocol {
	// case corev1.ProtocolUDP:
	// 	return global.GlobalPoolHealthCheckProtocolPINGUDP
	// }

	switch strings.TrimSpace(strings.ToUpper(pMonitorProtocol)) {
	case string(global.GlobalPoolHealthCheckProtocolHTTP):
		return global.GlobalPoolHealthCheckProtocolHTTP
	case string(global.GlobalPoolHealthCheckProtocolHTTPs):
		return global.GlobalPoolHealthCheckProtocolHTTPs
		// case string(global.GlobalPoolHealthCheckProtocolPINGUDP):
		// 	return global.GlobalPoolHealthCheckProtocolPINGUDP
	}

	return global.GlobalPoolHealthCheckProtocolTCP
}

// ParsePoolProtocol parse string to pool protocol
func (l *modelBuilder) parsePoolProtocol(pPoolProtocol string) loadbalancerv2.PoolProtocol {
	opt := strings.TrimSpace(strings.ToUpper(pPoolProtocol))
	switch opt {
	case string(loadbalancerv2.PoolProtocolProxy):
		return loadbalancerv2.PoolProtocolProxy
	case string(loadbalancerv2.PoolProtocolHTTP):
		return loadbalancerv2.PoolProtocolHTTP
	case string(loadbalancerv2.PoolProtocolUDP):
		return loadbalancerv2.PoolProtocolUDP
	}
	return loadbalancerv2.PoolProtocolTCP
}

func (l *modelBuilder) coreProtocolToSecgroupProtocol(protocol corev1.Protocol) networkv2.SecgroupRuleProtocol {
	switch protocol {
	case corev1.ProtocolTCP:
		return networkv2.SecgroupRuleProtocolTCP
	case corev1.ProtocolUDP:
		return networkv2.SecgroupRuleProtocolUDP
	default:
		return networkv2.SecgroupRuleProtocolTCP
	}
}
