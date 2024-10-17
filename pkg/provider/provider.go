package provider

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
)

func PointerOf[T any](t T) *T {
	return &t
}

// Provider is an interface that defines the methods that a provider must implement.
type Provider interface {
	Init(providerIDs []string) error
	GetProjectID() string
	GetNetworkID() string
	GetSubnetID() string
	GetSubnetCIDR() string
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

	// GetSubnet(subnetID string) (*objects.Subnet, error)

	GetServerByID(ctx context.Context, serverID string) (*entityv2.Server, error)
	WaitForServerActive(ctx context.Context, serverID string) error
	ListServerBySecgroupID(ctx context.Context, secgroupID string) (*entityv2.ListServers, error)

	ListLoadBalancers(ctx context.Context) (*entityv2.ListLoadBalancers, error)
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

	// GetPoolByName(lbID, name string) (*objects.Pool, error)
	// GetPoolByID(lbID, poolID string) (*entityv2.Pool, error)
	CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error)
	ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error)
	UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error
	GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error)
	DeletePool(ctx context.Context, lbID, poolID string) error
	UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error
	GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error)

	// ImportCertificate(opt *certificates.ImportOpts) (*objects.Certificate, error)
	// ListCertificates() ([]*objects.Certificate, error)
	// GetCertificateByID(certID string) (*objects.Certificate, error)
	// DeleteCertificate(certID string) error
}
