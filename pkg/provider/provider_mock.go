package provider

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)

func randID() string {
	number := randRange(1000000, 3999999)
	return fmt.Sprint(number)
}

func randIP() string {
	return fmt.Sprintf("1.1.%d.%d", randRange(1, 255), randRange(1, 255))
}

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}

var _ Provider = &MockProvider{}

type wrapListener struct {
	*entityv2.Listener
	lbID string
}

type wrapPool struct {
	*entityv2.Pool
	lbID string
}
type wrapPolicy struct {
	*entityv2.Policy
	lbID       string
	listenerID string
}

type MockProvider struct {
	// securityGroups []*objects.Secgroup
	projectID  string
	netID      string
	subnetID   string
	subnetCIDR string

	logger *logrus.Entry

	loadBalancers []*entityv2.LoadBalancer
	listeners     []*wrapListener
	pools         []*wrapPool
	policies      []*wrapPolicy

	mu sync.Mutex
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		projectID:  "projectID",
		netID:      "netID",
		subnetID:   "subnetID",
		subnetCIDR: "subnetCIDR",

		logger:        logrus.WithField("provider", "mock"),
		loadBalancers: make([]*entityv2.LoadBalancer, 0),
		listeners:     make([]*wrapListener, 0),
		pools:         make([]*wrapPool, 0),
		policies:      make([]*wrapPolicy, 0),
	}
}

func (m *MockProvider) Init(providerIDs []string) error {
	return nil
}

func (m *MockProvider) GetProjectID() string {
	return m.projectID
}

func (m *MockProvider) GetNetworkID() string {
	return m.netID
}

func (m *MockProvider) GetSubnetID() string {
	return m.subnetID
}

func (m *MockProvider) GetSubnetCIDR() string {
	return m.subnetCIDR
}

// // --------------------------- Security Group ---------------------------

// func (m *MockProvider) ListSecurityGroups() ([]*objects.Secgroup, error) {
// 	m.logger.Error("not implemented yet", "ListSecurityGroups")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) UpdateSecGroupsOfServer(instanceID string, secgroups []string) (*objects.Server, error) {
// 	m.logger.Error("not implemented yet", "UpdateSecGroupsOfServer")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) GetSecurityGroup(secgroupID string) (*objects.Secgroup, error) {
// 	m.logger.Error("not implemented yet", "GetSecurityGroup")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) DeleteSecurityGroup(secgroupID string) error {
// 	m.logger.Error("not implemented yet", "DeleteSecurityGroup")
// 	return errs.ErrorNotImplemented
// }
// func (m *MockProvider) CreateSecurityGroup(name string, description string) (*objects.Secgroup, error) {
// 	m.logger.Error("not implemented yet", "CreateSecurityGroup")
// 	return nil, errs.ErrorNotImplemented
// }

// func (m *MockProvider) CreateSecurityGroupRule(secgroupID string, opts *secgroup_rule.CreateOpts) (*objects.SecgroupRule, error) {
// 	m.logger.Error("not implemented yet", "CreateSecurityGroupRule")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) DeleteSecurityGroupRule(secgroupID string, ruleID string) error {
// 	m.logger.Error("not implemented yet", "DeleteSecurityGroupRule")
// 	return errs.ErrorNotImplemented
// }
// func (m *MockProvider) ListSecurityGroupRules(secgroupID string) ([]*objects.SecgroupRule, error) {
// 	m.logger.Error("not implemented yet", "ListSecurityGroupRules")
// 	return nil, errs.ErrorNotImplemented
// }

// // --------------------------- Tags ---------------------------

// func (m *MockProvider) GetTags(resourceID string) ([]*objects.ResourceTag, error) {
// 	m.logger.Error("not implemented yet", "GetTags")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) UpdateTags(resourceID string, tags map[string]string) error {
// 	m.logger.Error("not implemented yet", "UpdateTags")
// 	return errs.ErrorNotImplemented
// }

// func (m *MockProvider) GetSubnet(subnetID string) (*objects.Subnet, error) {
// 	m.logger.Error("not implemented yet", "GetSubnet")
// 	return nil, errs.ErrorNotImplemented
// }

// // --------------------------- Server ---------------------------

// func (m *MockProvider) GetServerByID(serverID string) (*objects.Server, error) {
// 	m.logger.Error("not implemented yet", "GetServerByID")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) ListServerByProviderIDs(providerIDs []string) ([]*objects.Server, error) {
// 	m.logger.Error("not implemented yet", "ListServerByProviderIDs")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) WaitForServerActive(serverID string) {

// }

// --------------------------- Load Balancer ---------------------------

func (m *MockProvider) ListLoadBalancers() (*entityv2.ListLoadBalancers, error) {
	lbs := &entityv2.ListLoadBalancers{
		Items:     m.loadBalancers,
		Page:      1,
		PageSize:  len(m.loadBalancers),
		TotalPage: 1,
		TotalItem: len(m.loadBalancers),
	}
	return lbs, nil
}
func (m *MockProvider) GetLoadBalancerByID(lbID string) (*entityv2.LoadBalancer, error) {
	for _, lb := range m.loadBalancers {
		if lb.GetId() == lbID {
			return lb, nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) GetLoadBalancerByName(name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers()
	if err != nil {
		return nil, err
	}
	for _, lb := range allLBs.Items {
		if lb.Name == name {
			return lb, nil
		}
	}
	return nil, nil
}

func (m *MockProvider) CreateLoadBalancer(lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	if lbOptions == nil {
		return nil, errs.ErrorInvalidInput
	}

	var lbOpt *loadbalancerv2.CreateLoadBalancerRequest
	if opt, ok := lbOptions.(*loadbalancerv2.CreateLoadBalancerRequest); ok {
		lbOpt = opt
	} else {
		return nil, errs.ErrorInvalidInput
	}

	lbID := "lb-" + randID()
	newLB := &entityv2.LoadBalancer{
		UUID:               lbID,
		Name:               lbOpt.Name,
		Address:            randIP(),
		Type:               string(lbOpt.Type),
		LoadBalancerSchema: string(lbOpt.Scheme),
		PackageID:          lbOpt.PackageID,
		SubnetID:           lbOpt.SubnetID,
		DisplayStatus:      consts.CREATED_LOADBALANCER_STATUS,
		PrivateSubnetID:    "????????",
		PrivateSubnetCidr:  "????????",
		DisplayType:        consts.CREATED_LOADBALANCER_STATUS,
		Description:        "????????",
		Location:           "????????",
		CreatedAt:          time.Now().Format(time.RFC3339),
		UpdatedAt:          time.Now().Format(time.RFC3339),
		ProgressStatus:     consts.CREATED_LOADBALANCER_STATUS,
		Status:             consts.ACTIVE_LOADBALANCER_STATUS,
		Internal:           lbOpt.Scheme == loadbalancerv2.InternalLoadBalancerScheme,
	}

	m.mu.Lock()
	m.loadBalancers = append(m.loadBalancers, newLB)
	m.mu.Unlock()

	defaultPoolID := ""
	if lbOpt.Pool != nil {
		pool, _ := m.CreatePool(lbID, lbOpt.Pool.WithLoadBalancerId(newLB.UUID))
		defaultPoolID = pool.UUID
	}

	if lbOpt.Listener != nil {
		m.CreateListener(lbID, lbOpt.Listener.WithLoadBalancerId(newLB.UUID).WithDefaultPoolId(defaultPoolID))
	}

	go func(o *entityv2.LoadBalancer) {
		time.Sleep(5 * time.Second)
		m.mu.Lock()
		o.DisplayStatus = consts.ACTIVE_LOADBALANCER_STATUS
		o.ProgressStatus = consts.CREATED_LOADBALANCER_STATUS
		m.mu.Unlock()
	}(newLB)

	return newLB, nil
}

func (m *MockProvider) DeleteLoadBalancer(lbID string) error {
	newListeners := make([]*wrapListener, 0)
	for i, lb := range m.listeners {
		if lb.lbID != lbID {
			newListeners = append(newListeners, m.listeners[i])
		}
	}
	m.listeners = newListeners

	newPools := make([]*wrapPool, 0)
	for i, lb := range m.pools {
		if lb.lbID != lbID {
			newPools = append(newPools, m.pools[i])
		}
	}
	m.pools = newPools

	m.mu.Lock()
	defer m.mu.Unlock()

	newLBs := make([]*entityv2.LoadBalancer, 0)
	for i, lb := range m.loadBalancers {
		if lb.GetId() != lbID {
			newLBs = append(newLBs, m.loadBalancers[i])
		}
	}
	if len(newLBs) == len(m.loadBalancers) {
		return errs.ErrorNotFound
	}

	m.loadBalancers = newLBs
	return nil
}
func (m *MockProvider) ResizeLoadBalancer(lbID, packageID string) error {
	m.logger.Error("not implemented yet", "ResizeLoadBalancer")
	return errs.ErrorNotImplemented
}
func (m *MockProvider) WaitForLBActive(lbID string) (*entityv2.LoadBalancer, error) {
	m.logger.Infof("Waiting for load balancer %s to be ready", lbID)
	var resultLb *entityv2.LoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetLoadBalancerByID(lbID)
		if err != nil {
			m.logger.Errorf("Error getting load balancer %s when wait active: %v", lbID, err)
			return false, err
		}
		if strings.ToUpper(lb.DisplayStatus) == consts.ACTIVE_LOADBALANCER_STATUS &&
			strings.ToUpper(lb.ProgressStatus) == consts.CREATED_LOADBALANCER_STATUS {
			m.logger.Infof("Load balancer %s is ready", lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			m.logger.Errorf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, errs.ErrorLoadBalancerStatusError
		}

		m.logger.Infof("Load balancer %s is not ready yet, waiting...", lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		m.logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

// --------------------------- Listener ---------------------------

//	func (m *MockProvider) GetListenerByName(lbID, name string) (*objects.Listener, error) {
//		m.logger.Error("not implemented yet", "GetListenerByName")
//		return nil, errs.ErrorNotImplemented
//	}
//
//	func (m *MockProvider) GetListenerByPort(lbID string, port int) (*objects.Listener, error) {
//		m.logger.Error("not implemented yet", "GetListenerByPort")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreateListener(lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
	listener := opt.ToRequestBody().(*loadbalancerv2.CreateListenerRequest)
	newListener := &wrapListener{
		lbID: lbID,
		Listener: &entityv2.Listener{
			UUID:                            "lis-" + randID(),
			Name:                            listener.ListenerName,
			Description:                     "????????",
			Protocol:                        string(listener.ListenerProtocol),
			ProtocolPort:                    listener.ListenerProtocolPort,
			ConnectionLimit:                 0,
			DefaultPoolId:                   "",
			DefaultPoolName:                 "",
			TimeoutClient:                   listener.TimeoutClient,
			TimeoutMember:                   listener.TimeoutMember,
			TimeoutConnection:               listener.TimeoutConnection,
			AllowedCidrs:                    listener.AllowedCidrs,
			DisplayStatus:                   consts.ACTIVE_LOADBALANCER_STATUS,
			CreatedAt:                       time.Now().Format(time.RFC3339),
			UpdatedAt:                       time.Now().Format(time.RFC3339),
			ProgressStatus:                  consts.ACTIVE_LOADBALANCER_STATUS,
			Headers:                         nil,
			CertificateAuthorities:          nil,
			DefaultCertificateAuthority:     nil,
			ClientCertificateAuthentication: nil,
		},
	}
	if listener.DefaultPoolId != nil && *listener.DefaultPoolId != "" {
		newListener.DefaultPoolId = *listener.DefaultPoolId
		pool, _ := m.GetPoolByID(lbID, newListener.DefaultPoolId)
		if pool != nil {
			newListener.DefaultPoolName = pool.Name
		}
	}

	m.mu.Lock()
	m.listeners = append(m.listeners, newListener)
	m.mu.Unlock()
	return newListener.Listener, nil
}
func (m *MockProvider) ListListenerOfLB(lbID string) (*entityv2.ListListeners, error) {
	listeners := make([]*entityv2.Listener, 0)
	for _, l := range m.listeners {
		if l.lbID == lbID {
			listeners = append(listeners, l.Listener)
		}
	}
	return &entityv2.ListListeners{
		Items: listeners,
	}, nil
}
func (m *MockProvider) DeleteListener(lbID, listenerID string) error {
	isFound := false
	newListeners := make([]*wrapListener, 0)
	for i, l := range m.listeners {
		if l.lbID != lbID || l.GetId() != listenerID {
			newListeners = append(newListeners, m.listeners[i])
		} else {
			isFound = true
		}
	}
	if !isFound {
		return errs.ErrorNotFound
	}
	m.listeners = newListeners
	return nil
}
func (m *MockProvider) UpdateListener(lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error {
	updateOpt := opt.ToRequestBody().(*loadbalancerv2.UpdateListenerRequest)
	var listener *wrapListener
	for _, l := range m.listeners {
		if l.lbID == lbID && l.GetId() == listenerID {
			listener = l
			break
		}
	}
	if listener == nil {
		return errs.ErrorNotFound
	}
	listener.Listener.TimeoutClient = updateOpt.TimeoutClient
	listener.Listener.TimeoutConnection = updateOpt.TimeoutConnection
	listener.Listener.TimeoutMember = updateOpt.TimeoutMember
	listener.Listener.AllowedCidrs = updateOpt.AllowedCidrs
	return nil
}

// --------------------------- Policy ---------------------------

//	func (m *MockProvider) GetPolicyByName(lbID, listenerID, name string) (*objects.Policy, error) {
//		m.logger.Error("not implemented yet", "GetPolicyByName")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreatePolicy(lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error) {
	lb, err := m.GetLoadBalancerByID(lbID)
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, errs.ErrorNotFound
	}
	listeners, err := m.ListListenerOfLB(lbID)
	if err != nil {
		return nil, err
	}
	if listeners == nil {
		return nil, errs.ErrorNotFound
	}
	isFound := false
	for _, l := range listeners.Items {
		if l.GetId() == listenerID {
			isFound = true
			break
		}
	}
	if !isFound {
		return nil, errs.ErrorNotFound
	}

	policy := opt.ToRequestBody().(*loadbalancerv2.CreatePolicyRequest)
	newPolicy := &wrapPolicy{
		lbID:       lbID,
		listenerID: listenerID,
		Policy: &entityv2.Policy{
			UUID:             "pol-" + randID(),
			Name:             policy.Name,
			Description:      "????????",
			RedirectPoolID:   policy.RedirectPoolID,
			Action:           string(policy.Action),
			RedirectURL:      policy.RedirectURL,
			RedirectPoolName: "",
			RedirectHTTPCode: policy.RedirectHTTPCode,
			KeepQueryString:  policy.KeepQueryString,
			L7Rules:          nil,
			Position:         0, // ????????
			DisplayStatus:    consts.ACTIVE_LOADBALANCER_STATUS,
			CreatedAt:        time.Now().Format(time.RFC3339),
			UpdatedAt:        time.Now().Format(time.RFC3339),
			ProgressStatus:   consts.ACTIVE_LOADBALANCER_STATUS,
		},
	}
	if policy.RedirectPoolID != "" {
		pool, _ := m.GetPoolByID(lbID, policy.RedirectPoolID)
		if pool != nil {
			newPolicy.RedirectPoolName = pool.Name
		} else {
			return nil, errs.ErrorNotFound
		}
	}
	newRules := make([]*entityv2.L7Rule, 0)
	for _, r := range policy.Rules {
		newRules = append(newRules, &entityv2.L7Rule{
			UUID:               "rule-" + randID(),
			CompareType:        string(r.CompareType),
			RuleValue:          r.RuleValue,
			RuleType:           string(r.RuleType),
			ProvisioningStatus: consts.ACTIVE_LOADBALANCER_STATUS,
			OperatingStatus:    consts.ACTIVE_LOADBALANCER_STATUS,
		})
	}
	newPolicy.L7Rules = newRules
	m.mu.Lock()
	m.policies = append(m.policies, newPolicy)
	m.mu.Unlock()
	return newPolicy.Policy, nil
}
func (m *MockProvider) ListPolicyOfListener(lbID, listenerID string) (*entityv2.ListPolicies, error) {
	policies := make([]*entityv2.Policy, 0)
	for _, p := range m.policies {
		if p.lbID == lbID && p.listenerID == listenerID {
			policies = append(policies, p.Policy)
		}
	}
	return &entityv2.ListPolicies{
		Items: policies,
	}, nil
}

//	func (m *MockProvider) GetPolicyByID(policyID string) (*objects.Policy, error) {
//		m.logger.Error("not implemented yet", "GetPolicyByID")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) UpdatePolicy(lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error {
	m.logger.Error("not implemented yet", "UpdatePolicy")
	return errs.ErrorNotImplemented
}
func (m *MockProvider) DeletePolicy(lbID, listenerID, policyID string) error {
	isFound := false
	newPolicies := make([]*wrapPolicy, 0)
	for i, p := range m.policies {
		if p.lbID != lbID || p.listenerID != listenerID || p.UUID != policyID {
			newPolicies = append(newPolicies, m.policies[i])
		} else {
			isFound = true
		}
	}
	if !isFound {
		return errs.ErrorNotFound
	}
	m.policies = newPolicies
	return nil
}

// --------------------------- Pool ---------------------------

//	func (m *MockProvider) GetPoolByName(lbID, name string) (*objects.Pool, error) {
//		m.logger.Error("not implemented yet", "GetPoolByName")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreatePool(lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
	var (
		pool          *loadbalancerv2.CreatePoolRequest
		healthMonitor *loadbalancerv2.HealthMonitor
		member        []*loadbalancerv2.Member
	)
	pool = opt.ToRequestBody().(*loadbalancerv2.CreatePoolRequest)
	defaultPoolID := "pool-" + randID()

	lb, _ := m.GetLoadBalancerByID(lbID)
	if lb == nil {
		return nil, errs.ErrorNotFound
	}

	newPool := &wrapPool{
		lbID: lbID,
		Pool: &entityv2.Pool{
			UUID:              defaultPoolID,
			Name:              pool.PoolName,
			Protocol:          string(pool.PoolProtocol),
			Description:       "????????",
			LoadBalanceMethod: string(pool.Algorithm),
			Status:            consts.ACTIVE_LOADBALANCER_STATUS,
			Stickiness:        false,
			TLSEncryption:     false,
			Members:           nil,
			HealthMonitor:     nil,
		},
	}

	if pool.HealthMonitor != nil {
		healthMonitor = pool.HealthMonitor.ToRequestBody().(*loadbalancerv2.HealthMonitor)
		newHealthMonitor := &entityv2.HealthMonitor{
			Timeout:             healthMonitor.Timeout,
			CreatedAt:           time.Now().Format(time.RFC3339),
			UpdatedAt:           time.Now().Format(time.RFC3339),
			DomainName:          healthMonitor.DomainName,
			Interval:            healthMonitor.Interval,
			HealthyThreshold:    healthMonitor.HealthyThreshold,
			UnhealthyThreshold:  healthMonitor.UnhealthyThreshold,
			HealthCheckPath:     healthMonitor.HealthCheckPath,
			SuccessCode:         healthMonitor.SuccessCode,
			ProgressStatus:      consts.ACTIVE_LOADBALANCER_STATUS,
			DisplayStatus:       consts.ACTIVE_LOADBALANCER_STATUS,
			HealthCheckProtocol: string(healthMonitor.HealthCheckProtocol),
			HttpVersion:         nil,
			HealthCheckMethod:   nil,
		}
		if healthMonitor.HttpVersion != nil {
			newHealthMonitor.HttpVersion = PointerOf(string(*healthMonitor.HttpVersion))
		}
		if healthMonitor.HealthCheckMethod != nil {
			newHealthMonitor.HealthCheckMethod = PointerOf(string(*healthMonitor.HealthCheckMethod))
		}
		newPool.HealthMonitor = newHealthMonitor
	}

	if pool.Members != nil {
		for _, m := range pool.Members {
			member = append(member, m.ToRequestBody().(*loadbalancerv2.Member))
		}
		newMembers := &entityv2.ListMembers{
			Items: make([]*entityv2.Member, 0),
		}
		for _, m := range member {
			newMember := &entityv2.Member{
				UUID:           "mem-" + randID(),
				Address:        m.IpAddress,
				ProtocolPort:   m.Port,
				Weight:         m.Weight,
				MonitorPort:    m.MonitorPort,
				SubnetID:       lb.SubnetID,
				Name:           m.Name,
				PoolID:         defaultPoolID,
				TypeCreate:     "????????",
				Backup:         false,
				DisplayStatus:  consts.ACTIVE_LOADBALANCER_STATUS,
				CreatedAt:      time.Now().Format(time.RFC3339),
				UpdatedAt:      time.Now().Format(time.RFC3339),
				CreatedBy:      "????????",
				ProgressStatus: consts.ACTIVE_LOADBALANCER_STATUS,
			}
			newMembers.Items = append(newMembers.Items, newMember)
		}
		newPool.Members = newMembers
	}

	m.mu.Lock()
	m.pools = append(m.pools, newPool)
	m.mu.Unlock()
	return newPool.Pool, nil
}
func (m *MockProvider) ListPool(lbID string) (*entityv2.ListPools, error) {
	pools := make([]*entityv2.Pool, 0)
	for _, p := range m.pools {
		if p.lbID == lbID {
			pools = append(pools, p.Pool)
		}
	}
	return &entityv2.ListPools{
		Items: pools,
	}, nil
}
func (m *MockProvider) UpdatePoolMembers(lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error {
	m.logger.Error("not implemented yet", "UpdatePoolMembers")
	return errs.ErrorNotImplemented
}

func (m *MockProvider) GetPoolByID(lbID, poolID string) (*entityv2.Pool, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return p.Pool, nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) GetPoolMembers(lbID, poolID string) (*entityv2.ListMembers, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return p.Members, nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) DeletePool(lbID, poolID string) error {
	isFound := false
	newPools := make([]*wrapPool, 0)
	for i, p := range m.pools {
		if p.lbID != lbID || p.GetId() != poolID {
			newPools = append(newPools, m.pools[i])
		} else {
			isFound = true
		}
	}
	if !isFound {
		return errs.ErrorNotFound
	}
	m.pools = newPools
	return nil
}

func (m *MockProvider) UpdatePool(lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error {
	updateOpt := opt.ToRequestBody().(*loadbalancerv2.UpdatePoolRequest)
	var pool *wrapPool
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			pool = p
			break
		}
	}
	if pool == nil {
		return errs.ErrorNotFound
	}
	pool.Pool.LoadBalanceMethod = string(updateOpt.Algorithm)
	return nil
}

func (m *MockProvider) GetPoolHealthMonitorById(lbID, poolID string) (*entityv2.HealthMonitor, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return p.HealthMonitor, nil
		}
	}
	return nil, errs.ErrorNotFound
}

// // --------------------------- Certificate ---------------------------

// func (m *MockProvider) ImportCertificate(opt *certificates.ImportOpts) (*objects.Certificate, error) {
// 	m.logger.Error("not implemented yet", "ImportCertificate")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) ListCertificates() ([]*objects.Certificate, error) {
// 	m.logger.Error("not implemented yet", "ListCertificates")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) GetCertificateByID(certID string) (*objects.Certificate, error) {
// 	m.logger.Error("not implemented yet", "GetCertificateByID")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) DeleteCertificate(certID string) error {
// 	m.logger.Error("not implemented yet", "DeleteCertificate")
// 	return errs.ErrorNotImplemented
// }
