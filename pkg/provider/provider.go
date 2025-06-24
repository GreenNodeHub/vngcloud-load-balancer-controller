package provider

import (
	"context"
	"errors"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
)

const (
	// hcm zone
	DEFAULT_L7_PACKAGE_ID = "lbp-f562b658-0fd4-4fa6-9c57-c1a803ccbf86"
	DEFAULT_L4_PACKAGE_ID = "lbp-96b6b072-aadb-4b58-9d5f-c16ad69d36aa"
)

var (
	ErrorInvalidInput            = errors.New("invalid input")
	ErrorNotImplemented          = errors.New("not implemented yet")
	ErrorNotFound                = errors.New("not found")
	ErrorLoadBalancerStatusError = errors.New("load balancer status is error")
)

func PointerOf[T any](t T) *T {
	return &t
}

// Provider is an interface that defines the methods that a provider must implement.
type Provider interface {
	Init(providerIDs []string) error
	GetProjectID() string
	GetNetworkID() string
	GetNetworkCIDR() string
	GetDefaultSubnetID() string
	GetDefaultSubnetCIDR() string
	GetDefaultZone() common.Zone
	// GetDefaultPackage() (string, string, error)
	GetDefaultPackageNetworkLB(zone string) string
	GetDefaultPackageApplicationLB(zone string) string

	GetServerNetworkInfo(ctx context.Context, serverID string) (zoneID common.Zone, subnetID, subnetCIDR string, err error)

	// GetAllSubnetIDs returns all subnet CIRDs for the given provider IDs.
	GetAllSubnetCIRDs(ctx context.Context, providerIDs []string) ([]string, error)
	// clientServer
	// clientLoadBalancer
	// projectID
	// networkID

	ListSecurityGroups(ctx context.Context) (*entityv2.ListSecgroups, error)
	UpdateSecGroupsOfServer(ctx context.Context, instanceID string, secgroups []string) (*entityv2.Server, error) // should check if server is not active yet
	GetSecurityGroup(ctx context.Context, secgroupID string) (*entityv2.Secgroup, error)
	DeleteSecurityGroup(ctx context.Context, secgroupID string) error
	CreateSecurityGroup(ctx context.Context, name string, description string) (*entityv2.Secgroup, error)

	CreateSecurityGroupRule(ctx context.Context, secgroupID string, opts networkv2.ICreateSecgroupRuleRequest) (*entityv2.SecgroupRule, error)
	DeleteSecurityGroupRule(ctx context.Context, secgroupID string, ruleID string) error
	ListSecurityGroupRules(ctx context.Context, secgroupID string) (*entityv2.ListSecgroupRules, error)

	ListTags(ctx context.Context, resourceID string) (*entityv2.ListTags, error)
	// overwrites the tags of the resource
	CreateTags(ctx context.Context, resourceID string, tags map[string]string) error
	// adding or updating the tags to the resource
	UpdateTags(ctx context.Context, resourceID string, tags map[string]string) error

	GetSubnetByID(ctx context.Context, networkID, subnetID string) (*entityv2.Subnet, error)

	GetServerByID(ctx context.Context, serverID string) (*entityv2.Server, error)
	WaitForServerActive(ctx context.Context, serverID string) error
	ListServerBySecgroupID(ctx context.Context, secgroupID string) (*entityv2.ListServers, error)

	ListLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListLoadBalancers, error)
	GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error)
	GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error)
	CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, lbID string) error
	ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error
	WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error)

	// GetListenerByName(lbID, name string) (*objects.Listener, error)
	// GetListenerByPort(lbID string, port int) (*objects.Listener, error)
	CreateListener(ctx context.Context, lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error)
	ListListenerOfLB(ctx context.Context, lbID string) (*entityv2.ListListeners, error)
	DeleteListener(ctx context.Context, lbID, listenerID string) error
	UpdateListener(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error

	// GetPolicyByName(lbID, listenerID, name string) (*objects.Policy, error)
	// GetPolicyByID(policyID string) (*objects.Policy, error)
	CreatePolicy(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error)
	ListPolicyOfListener(ctx context.Context, lbID, listenerID string) (*entityv2.ListPolicies, error)
	UpdatePolicy(ctx context.Context, lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error
	DeletePolicy(ctx context.Context, lbID, listenerID, policyID string) error
	ReorderPolicies(ctx context.Context, lbID, listenerID string, policyIDs []string) error

	// GetPoolByName(lbID, name string) (*objects.Pool, error)
	// GetPoolByID(lbID, poolID string) (*entityv2.Pool, error)
	CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error)
	ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error)
	UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error
	GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error)
	DeletePool(ctx context.Context, lbID, poolID string) error
	UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error
	GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error)

	ListCertificates(ctx context.Context) (*entityv2.ListCertificates, error)
	GetCertificateByID(ctx context.Context, certID string) (*entityv2.Certificate, error)
	ImportCertificate(ctx context.Context, opt loadbalancerv2.ICreateCertificateRequest) (*entityv2.Certificate, error)
	DeleteCertificate(ctx context.Context, certID string) error

	ListGlobalLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListGlobalLoadBalancers, error)
	GetGlobalLoadBalancerByID(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error)
	GetGlobalLoadBalancerByName(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error)
	CreateGlobalLoadBalancer(ctx context.Context, glbOptions global.ICreateGlobalLoadBalancerRequest) (*entityv2.GlobalLoadBalancer, error)
	DeleteGlobalLoadBalancer(ctx context.Context, glbID string) error
	WaitGlobalLoadBalancerActive(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error)

	ListGlobalPools(ctx context.Context, glbID string) (*entityv2.ListGlobalPools, error)
	CreateGlobalPool(ctx context.Context, glbID string, opt global.ICreateGlobalPoolRequest) (*entityv2.GlobalPool, error)
	DeleteGlobalPool(ctx context.Context, glbID, poolID string) error
	UpdateGlobalPool(ctx context.Context, glbID, poolID string, opt global.IUpdateGlobalPoolRequest) error
	ListGlobalPoolMembers(ctx context.Context, glbID, poolID string) (*entityv2.ListGlobalPoolMembers, error)
	PatchGlobalPoolMember(ctx context.Context, glbID, poolID string, opt global.IPatchGlobalPoolMemberRequest) error

	ListGlobalListeners(ctx context.Context, glbID string) (*entityv2.ListGlobalListeners, error)
	CreateGlobalListener(ctx context.Context, glbID string, opt global.ICreateGlobalListenerRequest) (*entityv2.GlobalListener, error)
	DeleteGlobalListener(ctx context.Context, glbID, listenerID string) error
	UpdateGlobalListener(ctx context.Context, glbID, listenerID string, opt global.IUpdateGlobalListenerRequest) error
}
