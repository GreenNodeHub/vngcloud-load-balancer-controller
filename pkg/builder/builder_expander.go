package builder

import (
	"encoding/json"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
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

func (l *ListenerBuilderType) GetICreateListenerRequest() loadbalancerv2.ICreateListenerRequest {
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

type ProtocolType string

const (
	ProtocolTypeTCP  ProtocolType = "tcp"
	ProtocolTypeUDP  ProtocolType = "udp"
	ProtocolTypeICMP ProtocolType = "icmp"
)

type SecGroupRuleBuilder interface {
	GetName() string
	GetID() string
	SetName(name string)
	SetID(id string)

	GetProtocol() ProtocolType
	GetPortRangeMin() int
	GetPortRangeMax() int
}

var _ SecGroupRuleBuilder = &secGroupRuleBuilderType{}

type secGroupRuleBuilderType struct {
	commonBuilder
	Protocol     ProtocolType
	PortRangeMin int
	PortRangeMax int
	// secgroup_rule.CreateOpts
}

func (s *secGroupRuleBuilderType) GetProtocol() ProtocolType {
	return s.Protocol
}

func (s *secGroupRuleBuilderType) GetPortRangeMin() int {
	return s.PortRangeMin
}

func (s *secGroupRuleBuilderType) GetPortRangeMax() int {
	return s.PortRangeMax
}

// ------------------------------------------------------------
