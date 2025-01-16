package builder

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
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

	GetICreatePoolRequest() loadbalancerv2.ICreatePoolRequest
	loadbalancerv2.ICreatePoolRequest
}

// var _ PoolBuilder = &poolBuilderType{}

type poolBuilderType struct {
	Algorithm     loadbalancerv2.PoolAlgorithm  `json:"algorithm"`
	PoolProtocol  loadbalancerv2.PoolProtocol   `json:"poolProtocol"`
	Stickiness    bool                          `json:"stickiness,omitempty"`    // only for l7, l4 doesn't have this field => nil
	TLSEncryption bool                          `json:"tlsEncryption,omitempty"` // only for l7, l4 doesn't have this field => nil
	HealthMonitor *loadbalancerv2.HealthMonitor `json:"healthMonitor"`
	Members       []*loadbalancerv2.Member      `json:"members"`

	commonBuilder
	IsL4      bool // check then stickness can be nil
	isDeleted bool
}

func (p *poolBuilderType) GetICreatePoolRequest(lbID string) loadbalancerv2.ICreatePoolRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range p.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}
	r := &loadbalancerv2.CreatePoolRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{LoadBalancerId: lbID},
		Algorithm:          p.Algorithm,
		PoolName:           p.GetName(),
		PoolProtocol:       p.PoolProtocol,
		Stickiness:         nil,
		TLSEncryption:      nil,
		HealthMonitor:      p.HealthMonitor,
		Members:            convertMembers,
	}
	if !p.IsL4 {
		r.Stickiness = &p.Stickiness
		r.TLSEncryption = &p.TLSEncryption
	}
	return r
}

func (p *poolBuilderType) GetIUpdatePoolMembersRequest(lbID string) loadbalancerv2.IUpdatePoolMembersRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range p.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}
	return loadbalancerv2.NewUpdatePoolMembersRequest(lbID, p.GetID()).WithMembers(convertMembers...)
}

func (p *poolBuilderType) GetIMembersRequest() []loadbalancerv2.IMemberRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range p.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}
	return convertMembers
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
func (current *poolBuilderType) ComparePoolBuilder(lbID string, new *poolBuilderType) (*loadbalancerv2.UpdatePoolRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	healthMonitor := &loadbalancerv2.HealthMonitor{
		HealthyThreshold:    new.HealthMonitor.HealthyThreshold,
		UnhealthyThreshold:  new.HealthMonitor.UnhealthyThreshold,
		Interval:            new.HealthMonitor.Interval,
		Timeout:             new.HealthMonitor.Timeout,
		HealthCheckProtocol: new.HealthMonitor.HealthCheckProtocol,
		HealthCheckMethod:   new.HealthMonitor.HealthCheckMethod,
		HttpVersion:         new.HealthMonitor.HttpVersion,
		HealthCheckPath:     new.HealthMonitor.HealthCheckPath,
		DomainName:          new.HealthMonitor.DomainName,
		SuccessCode:         new.HealthMonitor.SuccessCode,
	}
	updateOptions := &loadbalancerv2.UpdatePoolRequest{
		PoolCommon: common.PoolCommon{
			PoolId: current.GetID(),
		},
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		Algorithm:     new.Algorithm,
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: healthMonitor,
	}
	if !new.IsL4 {
		updateOptions.Stickiness = &new.Stickiness
		updateOptions.TLSEncryption = &new.TLSEncryption
	}
	if current.Algorithm != new.Algorithm {
		message = append(message, fmt.Sprintf("algorithm (%s -> %s)", current.Algorithm, new.Algorithm))
		isNeedUpdate = true
	}
	if !new.IsL4 && current.Stickiness != new.Stickiness {
		message = append(message, fmt.Sprintf("stickiness (%t -> %t)", current.Stickiness, new.Stickiness))
		isNeedUpdate = true
	}
	if !new.IsL4 && current.TLSEncryption != new.TLSEncryption {
		message = append(message, fmt.Sprintf("tls encryption (%t -> %t)", current.TLSEncryption, new.TLSEncryption))
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

	if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {
		// domain may return nil
		if current.HealthMonitor.HealthCheckPath == nil || *current.HealthMonitor.HealthCheckPath != *new.HealthMonitor.HealthCheckPath ||
			current.HealthMonitor.DomainName == nil || *current.HealthMonitor.DomainName != *new.HealthMonitor.DomainName ||
			current.HealthMonitor.HttpVersion == nil || *current.HealthMonitor.HttpVersion != *new.HealthMonitor.HttpVersion ||
			current.HealthMonitor.HealthCheckMethod == nil || *current.HealthMonitor.HealthCheckMethod != *new.HealthMonitor.HealthCheckMethod ||
			current.HealthMonitor.SuccessCode == nil || *current.HealthMonitor.SuccessCode != *new.HealthMonitor.SuccessCode {
			isNeedUpdate = true
		}
	} else if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolTCP {

		healthMonitor.HealthCheckProtocol = loadbalancerv2.HealthCheckProtocolHTTP
		healthMonitor.HealthCheckPath = current.HealthMonitor.HealthCheckPath
		healthMonitor.DomainName = current.HealthMonitor.DomainName
		healthMonitor.HttpVersion = current.HealthMonitor.HttpVersion
		healthMonitor.HealthCheckMethod = current.HealthMonitor.HealthCheckMethod
	} else if current.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolTCP &&
		new.HealthMonitor.HealthCheckProtocol == loadbalancerv2.HealthCheckProtocolHTTP {

		healthMonitor.HealthCheckProtocol = loadbalancerv2.HealthCheckProtocolTCP
		healthMonitor.HealthCheckPath = nil
		healthMonitor.DomainName = nil
		healthMonitor.HttpVersion = nil
		healthMonitor.HealthCheckMethod = nil
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
	loadbalancerv2.ICreateListenerRequest
}

var _ ListenerBuilder = &ListenerBuilderType{}

type ListenerBuilderType struct {
	commonBuilder
	loadbalancerv2.CreateListenerRequest
	ReferPoolName string
	IsL4          bool

	isDeleted      bool
	policyBuilders []*policyBuilderType
}

func (l *ListenerBuilderType) SetPoolID(poolID string) {
	l.CreateListenerRequest.DefaultPoolId = &poolID
}

func (l *ListenerBuilderType) GetPoolName() string {
	return l.ReferPoolName
}

func (l *ListenerBuilderType) GetICreateListenerRequest() *loadbalancerv2.CreateListenerRequest {
	return &l.CreateListenerRequest
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
func (current *ListenerBuilderType) CompareListenerBuilder(lbID string, new *ListenerBuilderType) (*loadbalancerv2.UpdateListenerRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdateListenerRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: current.GetID(),
		},
		AllowedCidrs:                new.AllowedCidrs,
		TimeoutClient:               new.TimeoutClient,
		TimeoutMember:               new.TimeoutMember,
		TimeoutConnection:           new.TimeoutConnection,
		DefaultPoolId:               *new.DefaultPoolId,
		DefaultCertificateAuthority: nil,
		CertificateAuthorities:      nil,
		Headers:                     nil,
		ClientCertificate:           nil,
	}

	// set current value
	if !new.IsL4 {
		updateOptions.Headers = new.Headers
		if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {
			updateOptions.ClientCertificate = new.ClientCertificate
			updateOptions.DefaultCertificateAuthority = new.DefaultCertificateAuthority
			updateOptions.CertificateAuthorities = new.CertificateAuthorities
		}
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

	if *current.DefaultPoolId != *new.DefaultPoolId {
		message = append(message, fmt.Sprintf("default pool id (%s -> %s)", *current.DefaultPoolId, *new.DefaultPoolId))
		isNeedUpdate = true
	}

	if !new.IsL4 {

		// headers
		slices.Sort(*current.Headers)
		slices.Sort(*new.Headers)
		if !slices.Equal(*current.Headers, *new.Headers) {
			message = append(message, fmt.Sprintf("headers (%v -> %v)", *current.Headers, *new.Headers))
			isNeedUpdate = true
		}

		if new.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {

			// client certificate
			if !comparePointer(current.ClientCertificate, new.ClientCertificate) {
				message = append(message, fmt.Sprintf("client certificate (%v -> %v)",
					pointerToString(current.ClientCertificate), pointerToString(new.ClientCertificate)))
				isNeedUpdate = true
			}

			// default certificate authority
			if !comparePointer(current.DefaultCertificateAuthority, new.DefaultCertificateAuthority) {
				message = append(message, fmt.Sprintf("default certificate authority (%s -> %s)",
					pointerToString(current.DefaultCertificateAuthority), pointerToString(new.DefaultCertificateAuthority)))
				isNeedUpdate = true
			}

			// certificate authorities
			if (current.CertificateAuthorities == nil || new.CertificateAuthorities == nil) &&
				current.CertificateAuthorities != new.CertificateAuthorities {
				message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
				isNeedUpdate = true
			} else {
				// CertificateAuthorities is not nil
				if len(*current.CertificateAuthorities) != len(*new.CertificateAuthorities) {
					message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
					isNeedUpdate = true
				} else {
					for _, ca := range *new.CertificateAuthorities {
						if !slices.Contains(*current.CertificateAuthorities, ca) {
							message = append(message, fmt.Sprintf("certificate authorities (%v -> %v)", current.CertificateAuthorities, new.CertificateAuthorities))
							isNeedUpdate = true
							break
						}
					}
				}
			}
		}
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
