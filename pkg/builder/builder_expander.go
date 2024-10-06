package builder

import (
	"encoding/json"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
)

type commonBuilder struct {
	Name string
	ID   string
}

func (c *commonBuilder) GetName() string {
	return c.Name
}
func (c *commonBuilder) GetID() string {
	return c.ID
}
func (c *commonBuilder) SetName(name string) {
	c.Name = name
}
func (c *commonBuilder) SetID(id string) {
	c.ID = id
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
	PoolName      string                        `json:"poolName"`
	PoolProtocol  loadbalancerv2.PoolProtocol   `json:"poolProtocol"`
	Stickiness    *bool                         `json:"stickiness,omitempty"`    // only for l7, l4 doesn't have this field => nil
	TLSEncryption *bool                         `json:"tlsEncryption,omitempty"` // only for l7, l4 doesn't have this field => nil
	HealthMonitor *loadbalancerv2.HealthMonitor `json:"healthMonitor"`
	Members       []*loadbalancerv2.Member      `json:"members"`

	commonBuilder
	IsL4 bool // check then stickness can be nil
}

func (p *poolBuilderType) GetICreatePoolRequest() loadbalancerv2.ICreatePoolRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range p.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}
	return loadbalancerv2.NewCreatePoolRequest(p.PoolName, p.PoolProtocol).
		WithAlgorithm(p.Algorithm).
		WithHealthMonitor(p.HealthMonitor).
		WithMembers(convertMembers...)
}

func (p *poolBuilderType) GetIUpdatePoolMembersRequest() loadbalancerv2.IUpdatePoolMembersRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range p.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IpAddress, member.Port, member.MonitorPort))
	}
	return loadbalancerv2.NewUpdatePoolMembersRequest(p.ID, p.GetID()).WithMembers(convertMembers...)
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

var _ ListenerBuilder = &listenerBuilderType{}

type listenerBuilderType struct {
	commonBuilder
	loadbalancerv2.CreateListenerRequest
	ReferPoolName string
	IsL4          bool

	isDeleted bool
}

func (l *listenerBuilderType) SetPoolID(poolID string) {
	l.CreateListenerRequest.DefaultPoolId = &poolID
}

func (l *listenerBuilderType) GetPoolName() string {
	return l.ReferPoolName
}

func (l *listenerBuilderType) GetICreateListenerRequest() loadbalancerv2.ICreateListenerRequest {
	return &l.CreateListenerRequest
	// return loadbalancerv2.NewCreateListenerRequest(l.Name, l.ListenerProtocol, l.ListenerProtocolPort).
	// 	WithDefaultPoolId(*l.DefaultPoolId).
	// 	WithAllowedCidrs(l.AllowedCidrs).
	// 	WithTimeoutClient(l.TimeoutClient).
	// 	WithTimeoutConnection(l.TimeoutConnection).
	// 	WithTimeoutMember(l.TimeoutMember)
}

func (l *listenerBuilderType) String() string {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return "Error while marshalling"
	}
	return string(b)
}

func (l *listenerBuilderType) SetIsDeleted(isDeleted bool) {
	l.isDeleted = isDeleted
}

func (l *listenerBuilderType) IsDeleted() bool {
	return l.isDeleted
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
