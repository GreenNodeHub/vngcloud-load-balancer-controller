package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
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
	CreateLoadBalancerOptions() loadbalancerv2.ICreateLoadBalancerRequest

	// default subnet id of the network if not specified
	GetSubnetID() (subnetID string)
	GetNetworkID() (networkID string)
	GetSubnetCIDR() string

	GetTargetNodeLabels() map[string]string
	GetTargetType() TargetType

	StringFormat() string
	JSONFormat() string
	Print()

	GetManageAnnotation() map[string]string

	// GetNodeBySelector(selector map[string]string) ([]*corev1.Node, error)
}

var _ ModelBuilder = &modelBuilder{}

type rule struct {
	RuleType string `json:"type"`
	Compare  string `json:"compare"`
	Value    string `json:"value"`
}

type action struct {
	Action           string `json:"action"`
	RedirectURL      string `json:"redirectUrl"`
	RedirectHTTPCode int    `json:"redirectHttpCode"`
	KeepQueryString  bool   `json:"keepQueryString"`
}

type implementationSpecificConfig struct {
	Path   string `json:"path"`
	Rules  []rule `json:"rules"`
	Action action `json:"action"`
}

type headerConfig struct {
	Http  []string `json:"http"`
	Https []string `json:"https"`
}

type modelBuilder struct {
	// annotation configuration
	isIgnored                  bool
	idleTimeoutClient          int
	idleTimeoutMember          int
	idleTimeoutConnection      int
	inboundCIDRs               []string
	healthcheckProtocol        loadbalancerv2.HealthCheckProtocol
	healthcheckPath            string
	successCodes               string
	healthcheckHttpVersion     loadbalancerv2.HealthCheckHttpVersion
	healthcheckHttpDomainName  string
	healthyThresholdCount      int
	unhealthyThresholdCount    int
	poolAlgorithm              loadbalancerv2.PoolAlgorithm
	targetNodeLabels           map[string]string
	healthcheckPort            int
	healthcheckHttpMethod      loadbalancerv2.HealthCheckMethod
	healthcheckTimeoutSeconds  int
	healthcheckIntervalSeconds int
	enableAutoscale            bool
	targetType                 TargetType
	isPOC                      bool

	// annotation configuration for L4 only
	enableProxyProtocol []string

	// annotation configuration for L7 only
	enableStickySession           bool
	enableTLSEncryption           bool
	certificateIDs                []string
	implementationSpecificConfigs []implementationSpecificConfig
	headers                       headerConfig
	clientCertificateID           string

	// helper components
	annotationParser annotations.Parser
	context          context.Context
	client           client.Client
	logger           *logrus.Entry
	cniType          utils.CNIType

	networkID  string
	subnetID   string
	subnetCIDR string

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

func (l *modelBuilder) CreateLoadBalancerOptions() loadbalancerv2.ICreateLoadBalancerRequest {
	lbName := l.GetLoadBalancerDefaultName()
	if l.GetLoadBalancerName() != "" {
		lbName = l.GetLoadBalancerName()
	}
	opts := loadbalancerv2.NewCreateLoadBalancerRequest(lbName, l.GetPackageID(), l.GetSubnetID()).
		WithScheme(l.scheme).
		WithType(l.loadBalancerType).
		WithPoc(l.isPOC).
		WithAutoScalable(l.enableAutoscale)

	// if have pool, create first pool, but in L7, only create default pool in this step
	if len(l.GetPoolBuilders()) > 0 {
		poolBuilder := l.GetPoolBuilders()[0]
		if poolBuilder.IsL4 || (!poolBuilder.IsL4 && poolBuilder.GetName() == consts.DEFAULT_NAME_DEFAULT_POOL) {
			// Both listener and pool properties must be required (non null) or both are not required (null); <nil> map[]},
			// if have listener, create first listener
			if len(l.GetListenerBuilders()) > 0 {
				listenerBuilder := l.GetListenerBuilders()[0]
				opts.WithListener(
					listenerBuilder.GetICreateListenerRequest(),
				)
				opts.WithPool(poolBuilder.GetICreatePoolRequest(""))
			}
		}
	}

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

func (l *modelBuilder) GetManageAnnotation() map[string]string {
	poolInfos := make([]string, 0)
	for _, pool := range l.GetPoolBuilders() {
		if pool.IsDeleted() {
			continue
		}
		if pool.GetName() == "" || pool.GetID() == "" {
			l.logger.Warnf("Pool name: %s, ID: %s is empty.", pool.GetName(), pool.GetID())
		}
		poolInfos = append(poolInfos, fmt.Sprintf("%s:%s", pool.GetID(), pool.GetName()))
	}

	listenerInfos := make([]string, 0)
	for _, listener := range l.GetListenerBuilders() {
		if listener.IsDeleted() {
			continue
		}
		if listener.GetName() == "" || listener.GetID() == "" {
			l.logger.Warnf("Listener name: %s, ID: %s is empty.", listener.GetName(), listener.GetID())
		}
		policyInfos := make([]string, 0)
		for _, policy := range listener.GetPolicyBuilders() {
			if policy.IsDeleted() {
				continue
			}
			if policy.GetName() == "" {
				l.logger.Warnf("Policy name is empty.")
			}
			policyInfos = append(policyInfos, policy.GetName())
		}
		listenerInfos = append(listenerInfos, fmt.Sprintf("%s:%s:[%s]", listener.GetID(), listener.GetName(), strings.Join(policyInfos, "|")))
	}

	defaultPoolMembers := []string{}
	defaultPool := l.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL)
	if defaultPool != nil {
		for _, member := range defaultPool.Members {
			defaultPoolMembers = append(defaultPoolMembers,
				fmt.Sprintf("%s:%d:%d", member.IpAddress, member.Port, member.MonitorPort))
		}
	}

	return map[string]string{
		fmt.Sprintf("%s/%s", l.annotationParser.GetPrefix(), annotations.SuffixManagePools):      strings.Join(poolInfos, ","),
		fmt.Sprintf("%s/%s", l.annotationParser.GetPrefix(), annotations.SuffixManageListeners):  strings.Join(listenerInfos, ","),
		fmt.Sprintf("%s/%s", l.annotationParser.GetPrefix(), annotations.SuffixManageDFPMembers): strings.Join(defaultPoolMembers, ","),
	}
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
		Description:    "",
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
// func (l *modelBuilder)
