package builder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/global"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP       TargetType = "ip"
)

type MemberAddress struct {
	IP   string
	Name string
}

// just build the model, use to compare with the real load balancer
type ModelBuilder interface {
	BasicInfoHelper
	PoolListenerHelper
	NameHelper

	// manage security group
	IsCreateDefaultSecgroup() bool
	GetSecurityGroupIDs() []string
	GetListDefaultSecgroupRules() []*secGroupRuleBuilderType
	EnsureSecgroupPING_UDP() // if use UDP protocol, must open ICMP protocol too

	IsIgnored() bool
	// It mays create lb, listener, pool (not policy) at the same time
	CreateLoadBalancerOptions() global.ICreateGlobalLoadBalancerRequest

	// default subnet id of the network if not specified
	GetSubnetID() (subnetID string)
	GetNetworkID() (networkID string)
	GetSubnetCIDR() string

	GetTargetNodeLabels() map[string]string
	GetTargetType() TargetType

	StringFormat() string
	JSONFormat() string
	Print()

	// GetManageAnnotation() map[string]string

	GetConfigClusterID() string

	// GetNodeBySelector(selector map[string]string) ([]*corev1.Node, error)
}

var _ ModelBuilder = &modelBuilder{}

type headerConfig struct {
	Http  []string `json:"http"`
	Https []string `json:"https"`
}

type modelBuilder struct {
	// annotation configuration
	configClusterID            string
	isIgnored                  bool
	idleTimeoutClient          int
	idleTimeoutMember          int
	idleTimeoutConnection      int
	inboundCIDRs               []string
	healthcheckProtocol        global.GlobalPoolHealthCheckProtocol
	healthcheckPath            string
	successCodes               string
	healthcheckHttpVersion     global.GlobalPoolHealthCheckHttpVersion
	healthcheckHttpDomainName  string
	healthyThresholdCount      int
	unhealthyThresholdCount    int
	poolAlgorithm              global.GlobalPoolAlgorithm
	targetNodeLabels           map[string]string
	healthcheckPort            int
	healthcheckHttpMethod      global.GlobalPoolHealthCheckMethod
	healthcheckTimeoutSeconds  int
	healthcheckIntervalSeconds int
	enableAutoscale            bool
	targetType                 TargetType
	isPOC                      bool

	// annotation configuration for L4 only
	enableProxyProtocol []string

	// annotation configuration for L7 only
	enableStickySession bool
	enableTLSEncryption bool
	headers             headerConfig

	// helper components
	annotationParser annotations.Parser
	context          context.Context
	client           client.Client
	logger           *logrus.Entry
	cniType          utils.CNIType

	networkID  string
	subnetID   string
	subnetCIDR string
	region     string

	// if user pass the security group annotation, don't create any security group, just use the given security group
	isAutoCreateSecurityGroup bool
	securityGroups            []string
	secGroupRuleBuilders      []*secGroupRuleBuilderType

	poolListenerHelper
	basicInfoHelper
	nameHelper
}

func (l *modelBuilder) GetNodeBySelector(selector map[string]string) ([]*corev1.Node, error) {
	nodeSelector := labels.SelectorFromSet(labels.Set(selector))
	nodeList := &corev1.NodeList{}
	if err := l.client.List(l.context, nodeList, client.MatchingLabelsSelector{Selector: nodeSelector}); err != nil {
		return nil, err
	}

	nodes := make([]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes = append(nodes, &nodeList.Items[i])
	}

	return nodes, nil
}

func (l *modelBuilder) IsIgnored() bool {
	return l.isIgnored
}

func (l *modelBuilder) CreateLoadBalancerOptions() global.ICreateGlobalLoadBalancerRequest {
	lbName := l.GetLoadBalancerDefaultName()
	if l.GetLoadBalancerName() != "" {
		lbName = l.GetLoadBalancerName()
	}
	if len(l.GetListenerBuilders()) == 0 || len(l.GetPoolBuilders()) == 0 {
		l.logger.Errorf("Listener or Pool is empty, cannot create load balancer.")
		return nil
	}
	opts := global.NewCreateGlobalLoadBalancerRequest(lbName).
		WithGlobalListener(l.GetListenerBuilders()[0].GetICreateListenerRequest()).
		WithGlobalPool(l.GetPoolBuilders()[0].GetICreatePoolRequest("")).WithPackage(l.packageID)

	// if have tags, add tags
	// ............................
	return opts
}

func (l *modelBuilder) GetNetworkID() string {
	return l.networkID
}

func (l *modelBuilder) GetSubnetID() string {
	return l.subnetID
}

func (l *modelBuilder) GetSubnetCIDR() string {
	return l.subnetCIDR
}

func (l *modelBuilder) GetTargetNodeLabels() map[string]string {
	return l.targetNodeLabels
}

func (l *modelBuilder) GetTargetType() TargetType {
	return l.targetType
}

func (l *modelBuilder) StringFormat() string {
	return fmt.Sprintf("%+v", l)
}

func (l *modelBuilder) JSONFormat() string {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return "Error while marshalling"
	}
	return string(b)
}

func (l *modelBuilder) Print() {
	l.logger.Info(l.JSONFormat())
	// l.logger.Info(l.poolBuilders)
	// l.logger.Info(l.listenerBuilders)
}

// ---------------------------------------------------------- security group

func (l *modelBuilder) IsCreateDefaultSecgroup() bool {
	return l.isAutoCreateSecurityGroup
}

func (l *modelBuilder) GetSecurityGroupIDs() []string {
	return l.securityGroups
}

func (l *modelBuilder) GetListDefaultSecgroupRules() []*secGroupRuleBuilderType {
	return l.secGroupRuleBuilders
}

// only support add default secgroup rules for ingress
func (l *modelBuilder) addDefaultSecgroupRules(port int, protocol networkv2.SecgroupRuleProtocol) bool {
	isExist := false
	for _, rule := range l.secGroupRuleBuilders {
		if rule.PortRangeMax == port &&
			rule.PortRangeMin == port &&
			rule.Protocol == protocol {
			isExist = true
			break
		}
	}
	if isExist {
		return true
	}
	l.secGroupRuleBuilders = append(l.secGroupRuleBuilders, &secGroupRuleBuilderType{
		commonBuilder: commonBuilder{
			name: "",
			id:   "",
		},
		Description:    l.clusterID,
		Direction:      networkv2.SecgroupRuleDirectionIngress,
		EtherType:      networkv2.SecgroupRuleEtherTypeIPv4,
		PortRangeMax:   port,
		PortRangeMin:   port,
		Protocol:       protocol,
		RemoteIPPrefix: l.GetSubnetCIDR(),
	})
	return false
}

// ensurePING_UDP
func (l *modelBuilder) EnsureSecgroupPING_UDP() {
	for _, rule := range l.secGroupRuleBuilders {
		if rule.Protocol == networkv2.SecgroupRuleProtocolUDP {
			l.addDefaultSecgroupRules(rule.GetPortRangeMax(), networkv2.SecgroupRuleProtocolICMP)
		}
	}
}

func (l *modelBuilder) GetConfigClusterID() string {
	return l.configClusterID
}

// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
// func (l *modelBuilder)
