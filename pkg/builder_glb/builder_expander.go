package builder

import (
	"encoding/json"
	"fmt"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/global"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
)

type commonBuilder struct {
	name string
	id   string
}

func (c *commonBuilder) GetName() string {
	return c.name
}
func (c *commonBuilder) GetID() string {
	return c.id
}
func (c *commonBuilder) SetName(name string) {
	c.name = name
}
func (c *commonBuilder) SetID(id string) {
	c.id = id
}

// ------------------------------------------------------------

type PoolBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)

	GetICreatePoolRequest(string) global.ICreateGlobalPoolRequest
	// global.ICreateGlobalPoolRequest
}

var _ PoolBuilder = &poolBuilderType{}

type poolBuilderType struct {
	Algorithm         global.GlobalPoolAlgorithm         `json:"algorithm"`
	Description       string                             `json:"description,omitempty"`
	Name              string                             `json:"name"`
	Protocol          global.GlobalPoolProtocol          `json:"protocol"`
	Stickiness        *bool                              `json:"stickiness,omitempty"`    // only for l7, l4 doesn't have this field => nil
	TLSEncryption     *bool                              `json:"tlsEncryption,omitempty"` // only for l7, l4 doesn't have this field => nil
	HealthMonitor     *global.GlobalHealthMonitorRequest `json:"health"`
	GlobalPoolMembers []*poolMemberBuilderType           `json:"globalPoolMembers"`

	commonBuilder
	isDeleted bool
}

type poolMemberBuilderType struct {
	global.GlobalPoolMemberRequest
	id string
}

func (p *poolBuilderType) GetICreatePoolRequest(lbID string) global.ICreateGlobalPoolRequest {
	convertMembers := make([]global.ICreateGlobalPoolMemberRequest, 0)
	for _, member := range p.GlobalPoolMembers {
		convertMembers = append(convertMembers, &global.GlobalPoolMemberRequest{
			Name:        member.Name,
			Description: member.Description,
			Region:      member.Region,
			TrafficDial: member.TrafficDial,
			VPCID:       member.VPCID,
			Members:     member.Members,
			Type:        global.GlobalPoolMemberTypePrivate,
		})
	}
	r := &global.CreateGlobalPoolRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{LoadBalancerId: lbID},
		Algorithm:          p.Algorithm,
		Stickiness:         nil,
		TLSEncryption:      nil,
		HealthMonitor:      p.HealthMonitor,
		Description:        p.Description,
		Name:               p.Name,
		Protocol:           p.Protocol,
		GlobalPoolMembers:  convertMembers,
	}
	return r
}

func (p *poolBuilderType) String() string {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "Error while marshalling"
	}
	return string(b)
}

func (p *poolBuilderType) SetIsDeleted(isDeleted bool) {
	p.isDeleted = isDeleted
}

func (p *poolBuilderType) IsDeleted() bool {
	return p.isDeleted
}

// ComparePoolBuilder compares two pools.
func (current *poolBuilderType) ComparePoolBuilder(lbID string, new *poolBuilderType) (*global.UpdateGlobalPoolRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	healthMonitor := &global.GlobalHealthMonitorRequest{
		HealthyThreshold:    new.HealthMonitor.HealthyThreshold,
		UnhealthyThreshold:  new.HealthMonitor.UnhealthyThreshold,
		Interval:            new.HealthMonitor.Interval,
		Timeout:             new.HealthMonitor.Timeout,
		HealthCheckProtocol: new.HealthMonitor.HealthCheckProtocol,
		HttpMethod:          new.HealthMonitor.HttpMethod,
		HttpVersion:         new.HealthMonitor.HttpVersion,
		Path:                new.HealthMonitor.Path,
		DomainName:          new.HealthMonitor.DomainName,
		SuccessCode:         new.HealthMonitor.SuccessCode,
	}
	updateOptions := &global.UpdateGlobalPoolRequest{
		PoolCommon: common.PoolCommon{
			PoolId: current.GetID(),
		},
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		Algorithm:     new.Algorithm,
		HealthMonitor: healthMonitor,
	}

	if current.Algorithm != new.Algorithm {
		message = append(message, fmt.Sprintf("algorithm (%s -> %s)", current.Algorithm, new.Algorithm))
		isNeedUpdate = true
	}

	if current.HealthMonitor.HealthyThreshold != new.HealthMonitor.HealthyThreshold {
		message = append(message, fmt.Sprintf("healthy threshold (%d -> %d)", current.HealthMonitor.HealthyThreshold, new.HealthMonitor.HealthyThreshold))
		isNeedUpdate = true
	}
	if current.HealthMonitor.UnhealthyThreshold != new.HealthMonitor.UnhealthyThreshold {
		message = append(message, fmt.Sprintf("unhealthy threshold (%d -> %d)", current.HealthMonitor.UnhealthyThreshold, new.HealthMonitor.UnhealthyThreshold))
		isNeedUpdate = true
	}
	if current.HealthMonitor.Interval != new.HealthMonitor.Interval {
		message = append(message, fmt.Sprintf("interval (%d -> %d)", current.HealthMonitor.Interval, new.HealthMonitor.Interval))
		isNeedUpdate = true
	}
	if current.HealthMonitor.Timeout != new.HealthMonitor.Timeout {
		message = append(message, fmt.Sprintf("timeout (%d -> %d)", current.HealthMonitor.Timeout, new.HealthMonitor.Timeout))
		isNeedUpdate = true
	}

	if current.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolHTTP {
		// domain may return nil
		if current.HealthMonitor.Path == nil || *current.HealthMonitor.Path != *new.HealthMonitor.Path ||
			current.HealthMonitor.DomainName == nil || *current.HealthMonitor.DomainName != *new.HealthMonitor.DomainName ||
			current.HealthMonitor.HttpVersion == nil || *current.HealthMonitor.HttpVersion != *new.HealthMonitor.HttpVersion ||
			current.HealthMonitor.HttpMethod == nil || *current.HealthMonitor.HttpMethod != *new.HealthMonitor.HttpMethod ||
			current.HealthMonitor.SuccessCode == nil || *current.HealthMonitor.SuccessCode != *new.HealthMonitor.SuccessCode {
			isNeedUpdate = true
		}
	} else if current.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolTCP {

		healthMonitor.HealthCheckProtocol = global.GlobalPoolHealthCheckProtocolHTTP
		healthMonitor.Path = current.HealthMonitor.Path
		healthMonitor.DomainName = current.HealthMonitor.DomainName
		healthMonitor.HttpVersion = current.HealthMonitor.HttpVersion
		healthMonitor.HttpMethod = current.HealthMonitor.HttpMethod
	} else if current.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolTCP &&
		new.HealthMonitor.HealthCheckProtocol == global.GlobalPoolHealthCheckProtocolHTTP {

		healthMonitor.HealthCheckProtocol = global.GlobalPoolHealthCheckProtocolTCP
		healthMonitor.Path = nil
		healthMonitor.DomainName = nil
		healthMonitor.HttpVersion = nil
		healthMonitor.HttpMethod = nil
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// ------------------------------------------------------------

type PolicyBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)
}

var _ PolicyBuilder = &policyBuilderType{}

type policyBuilderType struct {
	commonBuilder
	Action           loadbalancerv2.PolicyAction    `json:"action"`
	Rules            []loadbalancerv2.L7RuleRequest `json:"rules"`
	RedirectPoolID   string                         `json:"redirectPoolId"`
	RedirectURL      string                         `json:"redirectUrl"`
	RedirectHTTPCode int                            `json:"redirectHttpCode"`
	KeepQueryString  bool                           `json:"keepQueryString"`

	ReferPoolName string
	isDeleted     bool
}

func (p *policyBuilderType) GetICreatePolicyRequest(lbID, lisID string) loadbalancerv2.ICreatePolicyRequest {
	return loadbalancerv2.NewCreatePolicyRequest(lbID, lisID).
		WithRules(p.Rules...).
		WithRedirectPoolId(p.RedirectPoolID).
		WithAction(p.Action).
		WithName(p.GetName()).
		WithKeepQueryString(p.KeepQueryString).
		WithRedirectHTTPCode(p.RedirectHTTPCode).
		WithRedirectURL(p.RedirectURL)
}

func (l *policyBuilderType) IsDeleted() bool {
	return l.isDeleted
}

func (l *policyBuilderType) SetIsDeleted(isDeleted bool) {
	l.isDeleted = isDeleted
}

// ComparePolicyBuilder
func (current *policyBuilderType) ComparePolicyBuilder(lbID, lisID string, new *policyBuilderType) (*loadbalancerv2.UpdatePolicyRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdatePolicyRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: lisID,
		},
		PolicyCommon: common.PolicyCommon{
			PolicyId: current.GetID(),
		},
		Action:           new.Action,
		Rules:            new.Rules,
		KeepQueryString:  new.KeepQueryString,
		RedirectPoolID:   new.RedirectPoolID,
		RedirectURL:      new.RedirectURL,
		RedirectHTTPCode: new.RedirectHTTPCode,
	}
	if current.Action != new.Action {
		message = append(message, fmt.Sprintf("action (%s -> %s)", current.Action, new.Action))
		isNeedUpdate = true
	}

	// options for redirect to pool
	if new.Action == loadbalancerv2.PolicyActionREDIRECTTOPOOL && current.RedirectPoolID != new.RedirectPoolID {
		message = append(message, fmt.Sprintf("redirect pool id (%s -> %s)", current.RedirectPoolID, new.RedirectPoolID))
		isNeedUpdate = true
	}

	// options for redirect to url
	if new.Action == loadbalancerv2.PolicyActionREDIRECTTOURL {
		if current.RedirectURL != new.RedirectURL {
			message = append(message, fmt.Sprintf("redirect url (%s -> %s)", current.RedirectURL, new.RedirectURL))
			isNeedUpdate = true
		}
		if current.RedirectHTTPCode != new.RedirectHTTPCode {
			message = append(message, fmt.Sprintf("redirect http code (%d -> %d)", current.RedirectHTTPCode, new.RedirectHTTPCode))
			isNeedUpdate = true
		}
		if current.KeepQueryString != new.KeepQueryString {
			message = append(message, fmt.Sprintf("keep query string (%t -> %t)", current.KeepQueryString, new.KeepQueryString))
			isNeedUpdate = true
		}
	}

	if len(current.Rules) != len(new.Rules) {
		message = append(message, fmt.Sprintf("len(rules) (%d -> %d)", len(current.Rules), len(new.Rules)))
		isNeedUpdate = true
	} else {
		for _, rule := range new.Rules {
			if !current.checkIfL7RuleExist(current.Rules, rule) {
				message = append(message, fmt.Sprintf("rules (%v -> %v)", current.Rules, new.Rules))
				isNeedUpdate = true
				break
			}
		}
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

func (l *policyBuilderType) checkIfL7RuleExist(rules []loadbalancerv2.L7RuleRequest, rule loadbalancerv2.L7RuleRequest) bool {
	for _, r := range rules {
		if r.CompareType == rule.CompareType &&
			r.RuleType == rule.RuleType &&
			r.RuleValue == rule.RuleValue {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------

type ListenerBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)

	SetPoolID(poolID string)
	GetPoolName() string
	global.ICreateGlobalListenerRequest
}

var _ ListenerBuilder = &ListenerBuilderType{}

type ListenerBuilderType struct {
	commonBuilder
	global.CreateGlobalListenerRequest
	ReferPoolName string

	isDeleted      bool
	policyBuilders []*policyBuilderType
}

func (l *ListenerBuilderType) SetPoolID(poolID string) {
	l.CreateGlobalListenerRequest.GlobalPoolId = poolID
}

func (l *ListenerBuilderType) GetPoolName() string {
	return l.ReferPoolName
}

func (l *ListenerBuilderType) GetICreateListenerRequest() *global.CreateGlobalListenerRequest {
	return &l.CreateGlobalListenerRequest
	// return loadbalancerv2.NewCreateListenerRequest(l.Name, l.ListenerProtocol, l.ListenerProtocolPort).
	// 	WithDefaultPoolId(*l.DefaultPoolId).
	// 	WithAllowedCidrs(l.AllowedCidrs).
	// 	WithTimeoutClient(l.TimeoutClient).
	// 	WithTimeoutConnection(l.TimeoutConnection).
	// 	WithTimeoutMember(l.TimeoutMember)
}

func (l *ListenerBuilderType) String() string {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return "Error while marshalling"
	}
	return string(b)
}

func (l *ListenerBuilderType) SetIsDeleted(isDeleted bool) {
	l.isDeleted = isDeleted
}

func (l *ListenerBuilderType) IsDeleted() bool {
	return l.isDeleted
}

func (l *ListenerBuilderType) GetPolicyBuilders() []*policyBuilderType {
	return l.policyBuilders
}

func (l *ListenerBuilderType) GetPolicyBuilderByName(name string) *policyBuilderType {
	for _, policyBuilder := range l.policyBuilders {
		if policyBuilder.GetName() == name {
			return policyBuilder
		}
	}
	return nil
}

// CompareListenerBuilder compares two listener options.
func (current *ListenerBuilderType) CompareListenerBuilder(lbID string, new *ListenerBuilderType) (*global.UpdateGlobalListenerRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &global.UpdateGlobalListenerRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: current.GetID(),
		},
		AllowedCidrs:      new.AllowedCidrs,
		TimeoutClient:     new.TimeoutClient,
		TimeoutMember:     new.TimeoutMember,
		TimeoutConnection: new.TimeoutConnection,
		Headers:           nil,
		GlobalPoolId:      new.GlobalPoolId,
	}

	if current.AllowedCidrs != new.AllowedCidrs {
		message = append(message, fmt.Sprintf("allowed cidrs (%v -> %v)", current.AllowedCidrs, new.AllowedCidrs))
		isNeedUpdate = true
	}

	if current.TimeoutClient != new.TimeoutClient {
		message = append(message, fmt.Sprintf("timeout client (%d -> %d)", current.TimeoutClient, new.TimeoutClient))
		isNeedUpdate = true
	}

	if current.TimeoutMember != new.TimeoutMember {
		message = append(message, fmt.Sprintf("timeout member (%d -> %d)", current.TimeoutMember, new.TimeoutMember))
		isNeedUpdate = true
	}

	if current.TimeoutConnection != new.TimeoutConnection {
		message = append(message, fmt.Sprintf("timeout connection (%d -> %d)", current.TimeoutConnection, new.TimeoutConnection))
		isNeedUpdate = true
	}

	if current.GlobalPoolId != new.GlobalPoolId {
		message = append(message, fmt.Sprintf("default pool id (%s -> %s)", current.GlobalPoolId, new.GlobalPoolId))
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// ------------------------------------------------------------

type CertificateBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)
}

var _ CertificateBuilder = &certificateBuilderType{}

type certificateBuilderType struct {
	commonBuilder
}

// ------------------------------------------------------------

type SecGroupRuleBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)

	GetProtocol() networkv2.SecgroupRuleProtocol
	GetPortRangeMin() int
	GetPortRangeMax() int

	GetICreateSecgroupRuleRequest(secgroupID string) *networkv2.CreateSecgroupRuleRequest
}

var _ SecGroupRuleBuilder = &secGroupRuleBuilderType{}

type secGroupRuleBuilderType struct {
	commonBuilder
	Description    string                          `json:"description"`
	Direction      networkv2.SecgroupRuleDirection `json:"direction"`
	EtherType      networkv2.SecgroupRuleEtherType `json:"etherType"`
	PortRangeMax   int                             `json:"portRangeMax"`
	PortRangeMin   int                             `json:"portRangeMin"`
	Protocol       networkv2.SecgroupRuleProtocol  `json:"protocol"`
	RemoteIPPrefix string                          `json:"remoteIpPrefix"`
}

func (s *secGroupRuleBuilderType) GetProtocol() networkv2.SecgroupRuleProtocol {
	return s.Protocol
}

func (s *secGroupRuleBuilderType) GetPortRangeMin() int {
	return s.PortRangeMin
}

func (s *secGroupRuleBuilderType) GetPortRangeMax() int {
	return s.PortRangeMax
}

func (s *secGroupRuleBuilderType) GetICreateSecgroupRuleRequest(secgroupID string) *networkv2.CreateSecgroupRuleRequest {
	return &networkv2.CreateSecgroupRuleRequest{
		SecurityGroupID: secgroupID,
		SecgroupCommon: networkv2.SecgroupCommon{
			SecgroupId: secgroupID,
		},
		Description:    s.Description,
		Direction:      s.Direction,
		EtherType:      s.EtherType,
		PortRangeMax:   s.PortRangeMax,
		PortRangeMin:   s.PortRangeMin,
		Protocol:       s.Protocol,
		RemoteIPPrefix: s.RemoteIPPrefix,
	}
}

// ------------------------------------------------------------
