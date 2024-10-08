package provider

import (
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
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

	// ListSecurityGroups() ([]*objects.Secgroup, error)
	// UpdateSecGroupsOfServer(instanceID string, secgroups []string) (*objects.Server, error)
	// GetSecurityGroup(secgroupID string) (*objects.Secgroup, error)
	// DeleteSecurityGroup(secgroupID string) error
	// CreateSecurityGroup(name string, description string) (*objects.Secgroup, error)

	// CreateSecurityGroupRule(secgroupID string, opts *secgroup_rule.CreateOpts) (*objects.SecgroupRule, error)
	// DeleteSecurityGroupRule(secgroupID string, ruleID string) error
	// ListSecurityGroupRules(secgroupID string) ([]*objects.SecgroupRule, error)

	// GetTags(resourceID string) ([]*objects.ResourceTag, error)
	// UpdateTags(resourceID string, tags map[string]string) error

	// GetSubnet(subnetID string) (*objects.Subnet, error)

	// GetServerByID(serverID string) (*objects.Server, error)
	// ListServerByProviderIDs(providerIDs []string) ([]*objects.Server, error)
	// WaitForServerActive(serverID string)

	ListLoadBalancers() (*entityv2.ListLoadBalancers, error)
	GetLoadBalancerByID(lbID string) (*entityv2.LoadBalancer, error)
	GetLoadBalancerByName(name string) (*entityv2.LoadBalancer, error)
	CreateLoadBalancer(lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error)
	DeleteLoadBalancer(lbID string) error
	ResizeLoadBalancer(lbID, packageID string) error
	WaitForLBActive(lbID string) (*entityv2.LoadBalancer, error)

	// GetListenerByName(lbID, name string) (*objects.Listener, error)
	// GetListenerByPort(lbID string, port int) (*objects.Listener, error)
	CreateListener(lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error)
	ListListenerOfLB(lbID string) (*entityv2.ListListeners, error)
	DeleteListener(lbID, listenerID string) error
	UpdateListener(lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error

	// GetPolicyByName(lbID, listenerID, name string) (*objects.Policy, error)
	// GetPolicyByID(policyID string) (*objects.Policy, error)
	CreatePolicy(lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error)
	ListPolicyOfListener(lbID, listenerID string) (*entityv2.ListPolicies, error)
	UpdatePolicy(lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error
	DeletePolicy(lbID, listenerID, policyID string) error

	// GetPoolByName(lbID, name string) (*objects.Pool, error)
	// GetPoolByID(lbID, poolID string) (*entityv2.Pool, error)
	CreatePool(lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error)
	ListPool(lbID string) (*entityv2.ListPools, error)
	UpdatePoolMembers(lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error
	GetPoolMembers(lbID, poolID string) (*entityv2.ListMembers, error)
	DeletePool(lbID, poolID string) error
	UpdatePool(lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error
	GetPoolHealthMonitorById(lbID, poolID string) (*entityv2.HealthMonitor, error)

	// ImportCertificate(opt *certificates.ImportOpts) (*objects.Certificate, error)
	// ListCertificates() ([]*objects.Certificate, error)
	// GetCertificateByID(certID string) (*objects.Certificate, error)
	// DeleteCertificate(certID string) error
}
