package provider

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	clone "github.com/huandu/go-clone"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)

const (
	MockProjectID  = "projectID"
	MockNetID      = "netID"
	MockSubnetID   = "subnetID"
	MockSubnetCIDR = "199.0.0.0/24"
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
type wrapServer struct {
	*entityv2.Server
}
type wrapSecgroup struct {
	*entityv2.Secgroup
}
type wrapSecgroupRule struct {
	*entityv2.SecgroupRule
}

type MockProvider struct {
	// securityGroups []*objects.Secgroup
	projectID  string
	netID      string
	subnetID   string
	subnetCIDR string

	loadBalancers []*entityv2.LoadBalancer
	listeners     []*wrapListener
	pools         []*wrapPool
	policies      []*wrapPolicy
	tags          map[string](map[string]string)
	servers       []*wrapServer
	secgroups     []*wrapSecgroup
	secgroupRules []*wrapSecgroupRule

	mu            sync.Mutex
	WaitAfterTime time.Duration
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		projectID:  MockProjectID,
		netID:      MockNetID,
		subnetID:   MockSubnetID,
		subnetCIDR: MockSubnetCIDR,

		loadBalancers: make([]*entityv2.LoadBalancer, 0),
		listeners:     make([]*wrapListener, 0),
		pools:         make([]*wrapPool, 0),
		policies:      make([]*wrapPolicy, 0),
		tags:          make(map[string](map[string]string)),
		servers:       make([]*wrapServer, 0),
		secgroups:     make([]*wrapSecgroup, 0),
		secgroupRules: make([]*wrapSecgroupRule, 0),

		WaitAfterTime: 0,
	}
}

func (m *MockProvider) Init(_ []string) error {
	// add server mock
	serverIDs := []string{
		"ins-00000000-0000-0000-0000-000000000001",
		"ins-00000000-0000-0000-0000-000000000002",
		"ins-00000000-0000-0000-0000-000000000003",
		"ins-00000000-0000-0000-0000-000000000004",
	}
	for _, id := range serverIDs {
		m.servers = append(m.servers, &wrapServer{
			Server: &entityv2.Server{
				Uuid:               id,
				BootVolumeId:       "",
				CreatedAt:          time.Now().Format(time.RFC3339),
				EncryptionVolume:   false,
				Licence:            false,
				Location:           "",
				Metadata:           "",
				MigrateState:       "",
				Name:               "mock-server-1",
				Product:            "",
				ServerGroupId:      "",
				ServerGroupName:    "",
				SshKeyName:         "",
				Status:             consts.ACTIVE_LOADBALANCER_STATUS,
				StopBeforeMigrate:  false,
				User:               "",
				Image:              entityv2.Image{},
				Flavor:             entityv2.Flavor{},
				SecGroups:          []entityv2.ServerSecgroup{},
				ExternalInterfaces: []entityv2.NetworkInterface{},
				InternalInterfaces: []entityv2.NetworkInterface{},
			},
		})
	}
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

func (m *MockProvider) ListSecurityGroups(ctx context.Context) (*entityv2.ListSecgroups, error) {
	secgroups := make([]*entityv2.Secgroup, 0)
	for _, s := range m.secgroups {
		secgroups = append(secgroups, clone.Clone(s.Secgroup).(*entityv2.Secgroup))
	}
	return &entityv2.ListSecgroups{
		Items: secgroups,
	}, nil
}

func (m *MockProvider) UpdateSecGroupsOfServer(ctx context.Context, instanceID string, secgroups []string) (*entityv2.Server, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update security groups of server %s", icon, instanceID)

	var server *entityv2.Server
	for _, s := range m.servers {
		if s.Uuid == instanceID {
			server = s.Server
			break
		}
	}
	if server == nil {
		return nil, errs.ErrorNotFound
	}

	// check secgroups is valid
	secG := make([]entityv2.ServerSecgroup, 0)
	for _, secgroupID := range secgroups {
		s, err := m.GetSecurityGroup(ctx, secgroupID)
		if err != nil {
			return nil, err
		}
		secG = append(secG, entityv2.ServerSecgroup{
			Uuid: s.Id,
			Name: s.Name,
		})
	}

	server.SecGroups = secG
	return clone.Clone(server).(*entityv2.Server), nil
}

func (m *MockProvider) GetSecurityGroup(ctx context.Context, secgroupID string) (*entityv2.Secgroup, error) {
	for _, s := range m.secgroups {
		if s.Id == secgroupID {
			return clone.Clone(s.Secgroup).(*entityv2.Secgroup), nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) DeleteSecurityGroup(ctx context.Context, secgroupID string) error {
	// valid secgroupID
	servers, err := m.ListServerBySecgroupID(ctx, secgroupID)
	if err != nil {
		return err
	}

	if len(servers.Items) > 0 {
		return errs.ErrorSecurityGroupInUse
	}

	// delete secgroup
	newSecgroups := make([]*wrapSecgroup, 0)
	for i, s := range m.secgroups {
		if s.Id != secgroupID {
			newSecgroups = append(newSecgroups, m.secgroups[i])
		}
	}

	m.mu.Lock()
	m.secgroups = newSecgroups
	m.mu.Unlock()
	return nil
}

func (m *MockProvider) CreateSecurityGroup(ctx context.Context, name string, description string) (*entityv2.Secgroup, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create security group %s", icon, name)

	secgroupID := "secgroup-" + randID()
	newSecgroup := &wrapSecgroup{
		&entityv2.Secgroup{
			Id:          secgroupID,
			Name:        name,
			Description: description,
			Status:      consts.ACTIVE_LOADBALANCER_STATUS,
		},
	}

	m.mu.Lock()
	m.secgroups = append(m.secgroups, newSecgroup)
	m.mu.Unlock()
	return clone.Clone(newSecgroup.Secgroup).(*entityv2.Secgroup), nil
}

func (m *MockProvider) CreateSecurityGroupRule(ctx context.Context, secgroupID string, opts networkv2.ICreateSecgroupRuleRequest) (*entityv2.SecgroupRule, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create security group rule for security group %s", icon, secgroupID)

	// valid secgroupID
	_, err := m.GetSecurityGroup(ctx, secgroupID)
	if err != nil {
		return nil, err
	}

	rule := opts.(*networkv2.CreateSecgroupRuleRequest)
	newRule := &wrapSecgroupRule{
		&entityv2.SecgroupRule{
			Id:             "sec-rule-" + randID(),
			SecgroupId:     secgroupID,
			Direction:      string(rule.Direction),
			EtherType:      string(rule.EtherType),
			Protocol:       string(rule.Protocol),
			Description:    rule.Description,
			RemoteIPPrefix: rule.RemoteIPPrefix,
			PortRangeMax:   rule.PortRangeMax,
			PortRangeMin:   rule.PortRangeMin,
		},
	}

	m.mu.Lock()
	m.secgroupRules = append(m.secgroupRules, newRule)
	m.mu.Unlock()
	return clone.Clone(newRule.SecgroupRule).(*entityv2.SecgroupRule), nil
}

func (m *MockProvider) DeleteSecurityGroupRule(ctx context.Context, secgroupID string, ruleID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete security group rule %s of security group %s", icon, ruleID, secgroupID)

	// valid secgroupID
	_, err := m.GetSecurityGroup(ctx, secgroupID)
	if err != nil {
		return err
	}

	// delete rule
	isFound := false
	newRules := make([]*wrapSecgroupRule, 0)
	for i, r := range m.secgroupRules {
		if r.SecgroupId != secgroupID || r.Id != ruleID {
			newRules = append(newRules, m.secgroupRules[i])
		} else {
			isFound = true
		}
	}

	if !isFound {
		return errs.ErrorNotFound
	}

	m.mu.Lock()
	m.secgroupRules = newRules
	m.mu.Unlock()
	return nil
}

func (m *MockProvider) ListSecurityGroupRules(ctx context.Context, secgroupID string) (*entityv2.ListSecgroupRules, error) {
	// valid secgroupID
	_, err := m.GetSecurityGroup(ctx, secgroupID)
	if err != nil {
		return nil, err
	}

	rules := make([]*entityv2.SecgroupRule, 0)
	for _, r := range m.secgroupRules {
		if r.SecgroupId == secgroupID {
			rules = append(rules, clone.Clone(r.SecgroupRule).(*entityv2.SecgroupRule))
		}
	}
	return &entityv2.ListSecgroupRules{
		Items: rules,
	}, nil
}

// // --------------------------- Tags ---------------------------

func (m *MockProvider) ListTags(ctx context.Context, resourceID string) (*entityv2.ListTags, error) {
	tags := make(map[string]string)
	if t, ok := m.tags[resourceID]; ok {
		tags = t
	}

	tagItems := make([]*entityv2.Tag, 0)
	for k, v := range tags {
		tagItems = append(tagItems, &entityv2.Tag{
			Key:   k,
			Value: v,
		})
	}
	return &entityv2.ListTags{
		Items: tagItems,
	}, nil
}

func (m *MockProvider) CreateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create tags for resource %s", icon, resourceID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tags == nil {
		m.tags = make(map[string](map[string]string))
	}
	m.tags[resourceID] = tags
	return nil
}

func (m *MockProvider) UpdateTags(ctx context.Context, resourceID string, tags map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tags == nil {
		m.tags = make(map[string](map[string]string))
	}
	if m.tags[resourceID] == nil {
		m.tags[resourceID] = make(map[string]string)
	}
	for k, v := range tags {
		m.tags[resourceID][k] = v
	}
	return nil
}

// func (m *MockProvider) GetSubnet(ctx context.Context, subnetID string) (*objects.Subnet, error) {
// 	logger.Error("not implemented yet", "GetSubnet")
// 	return nil, errs.ErrorNotImplemented
// }

// // --------------------------- Server ---------------------------

func (m *MockProvider) GetServerByID(ctx context.Context, serverID string) (*entityv2.Server, error) {
	for _, s := range m.servers {
		if s.Uuid == serverID {
			return clone.Clone(s.Server).(*entityv2.Server), nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) WaitForServerActive(ctx context.Context, serverID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Waiting for server %s to be ready", waitIcon, serverID)

	var server *entityv2.Server
	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		var _err error
		server, _err = m.GetServerByID(ctx, serverID)
		if _err != nil {
			logger.Errorf("Error getting server %s when wait active: %v", serverID, _err)
			return false, _err
		}
		if strings.ToUpper(server.Status) == consts.ACTIVE_LOADBALANCER_STATUS {
			logger.Infof("%s Server %s is ready", readyIcon, serverID)
			return true, nil
		}
		if strings.ToUpper(server.Status) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Errorf("Server %s is in error status", serverID)
			return true, errs.ErrorLoadBalancerStatusError
		}

		logger.Infof("%s Server %s is not ready yet, waiting...", waitIcon, serverID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", serverID, server.Status)
	}

	return err
}

func (m *MockProvider) ListServerBySecgroupID(ctx context.Context, secgroupID string) (*entityv2.ListServers, error) {
	// valid secgroupID
	_, err := m.GetSecurityGroup(ctx, secgroupID)
	if err != nil {
		return nil, err
	}

	// get servers by secgroup
	servers := make([]*entityv2.Server, 0)
	for _, s := range m.servers {
		for _, sg := range s.SecGroups {
			if sg.Uuid == secgroupID {
				servers = append(servers, clone.Clone(s.Server).(*entityv2.Server))
				break
			}
		}
	}
	return &entityv2.ListServers{
		Items: servers,
	}, nil
}

// --------------------------- Load Balancer ---------------------------

func (m *MockProvider) ListLoadBalancers(ctx context.Context) (*entityv2.ListLoadBalancers, error) {
	lbs := &entityv2.ListLoadBalancers{
		Items:     m.loadBalancers,
		Page:      1,
		PageSize:  len(m.loadBalancers),
		TotalPage: 1,
		TotalItem: len(m.loadBalancers),
	}
	return lbs, nil
}
func (m *MockProvider) GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	for _, lb := range m.loadBalancers {
		if lb.GetId() == lbID {
			return clone.Clone(lb).(*entityv2.LoadBalancer), nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers(ctx)
	if err != nil {
		return nil, err
	}
	for _, lb := range allLBs.Items {
		if lb.Name == name {
			return clone.Clone(lb).(*entityv2.LoadBalancer), nil
		}
	}
	return nil, nil
}

func (m *MockProvider) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create load balancer.", icon)
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
		pool, _ := m.CreatePool(ctx, lbID, lbOpt.Pool.WithLoadBalancerId(newLB.UUID))
		defaultPoolID = pool.UUID
	}

	if lbOpt.Listener != nil {
		m.CreateListener(ctx, lbID, lbOpt.Listener.WithLoadBalancerId(newLB.UUID).WithDefaultPoolId(defaultPoolID))
	}

	m.updatingStatus(newLB.UUID)
	go m.readyAfterTime(newLB.UUID)

	return &entityv2.LoadBalancer{
		UUID: newLB.UUID,
	}, nil
}

func (m *MockProvider) updatingStatus(lbID string) {
	logger := contexts.NewContext(context.TODO()).Log()
	var o *entityv2.LoadBalancer
	for _, lb := range m.loadBalancers {
		if lb.GetId() == lbID {
			o = lb
			break
		}
	}
	if o == nil {
		logger.Error("Load Balancer not found")
		return
	}

	if m.WaitAfterTime == 0 {
		m.mu.Lock()
		o.DisplayStatus = consts.ACTIVE_LOADBALANCER_STATUS
		o.ProgressStatus = consts.CREATED_LOADBALANCER_STATUS
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	o.DisplayStatus = consts.CREATED_LOADBALANCER_STATUS
	o.ProgressStatus = consts.CREATED_LOADBALANCER_STATUS
	m.mu.Unlock()
}

func (m *MockProvider) readyAfterTime(lbID string) {
	if m.WaitAfterTime == 0 {
		return
	}
	logger := contexts.NewContext(context.TODO()).Log()
	var o *entityv2.LoadBalancer
	for _, lb := range m.loadBalancers {
		if lb.GetId() == lbID {
			o = lb
			break
		}
	}
	if o == nil {
		logger.Error("Load Balancer not found")
		return
	}

	time.Sleep(m.WaitAfterTime)
	m.mu.Lock()
	o.DisplayStatus = consts.ACTIVE_LOADBALANCER_STATUS
	o.ProgressStatus = consts.CREATED_LOADBALANCER_STATUS
	m.mu.Unlock()
}

func (m *MockProvider) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete load balancer %s", icon, lbID)
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
func (m *MockProvider) ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Error("not implemented yet", "ResizeLoadBalancer")
	return errs.ErrorNotImplemented
}
func (m *MockProvider) WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Waiting for load balancer %s to be ready", waitIcon, lbID)
	var resultLb *entityv2.LoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetLoadBalancerByID(ctx, lbID)
		if err != nil {
			logger.Errorf("Error getting load balancer %s when wait active: %v", lbID, err)
			return false, err
		}
		if strings.ToUpper(lb.DisplayStatus) == consts.ACTIVE_LOADBALANCER_STATUS &&
			strings.ToUpper(lb.ProgressStatus) == consts.CREATED_LOADBALANCER_STATUS {
			logger.Infof("%s Load balancer %s is ready", readyIcon, lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Errorf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, errs.ErrorLoadBalancerStatusError
		}

		logger.Infof("%s Load balancer %s is not ready yet, waiting...", waitIcon, lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for the loadbalancer %s with lb status %s", lbID, resultLb.Status)
	}

	return resultLb, err
}

// --------------------------- Listener ---------------------------

//	func (m *MockProvider) GetListenerByName(ctx context.Context, lbID, name string) (*objects.Listener, error) {
//		logger.Error("not implemented yet", "GetListenerByName")
//		return nil, errs.ErrorNotImplemented
//	}
//
//	func (m *MockProvider) GetListenerByPort(ctx context.Context, lbID string, port int) (*objects.Listener, error) {
//		logger.Error("not implemented yet", "GetListenerByPort")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreateListener(ctx context.Context, lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create listener of load balancer %s", icon, lbID)
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
		pool, _ := m.GetPoolByID(ctx, lbID, newListener.DefaultPoolId)
		if pool != nil {
			newListener.DefaultPoolName = pool.Name
		}
	}

	m.mu.Lock()
	m.listeners = append(m.listeners, newListener)
	m.mu.Unlock()

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return &entityv2.Listener{
		UUID: newListener.Listener.UUID,
	}, nil
}
func (m *MockProvider) ListListenerOfLB(ctx context.Context, lbID string) (*entityv2.ListListeners, error) {
	listeners := make([]*entityv2.Listener, 0)
	for _, l := range m.listeners {
		if l.lbID == lbID {
			listeners = append(listeners, clone.Clone(l.Listener).(*entityv2.Listener))
		}
	}
	return &entityv2.ListListeners{
		Items: listeners,
	}, nil
}
func (m *MockProvider) DeleteListener(ctx context.Context, lbID, listenerID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete listener %s of load balancer %s", icon, listenerID, lbID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}
func (m *MockProvider) UpdateListener(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update listener %s of load balancer %s", icon, listenerID, lbID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}

// --------------------------- Policy ---------------------------

//	func (m *MockProvider) GetPolicyByName(ctx context.Context, lbID, listenerID, name string) (*objects.Policy, error) {
//		logger.Error("not implemented yet", "GetPolicyByName")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreatePolicy(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create policy of listener %s of load balancer %s", icon, listenerID, lbID)
	lb, err := m.GetLoadBalancerByID(ctx, lbID)
	if err != nil {
		return nil, err
	}
	if lb == nil {
		return nil, errs.ErrorNotFound
	}
	listeners, err := m.ListListenerOfLB(ctx, lbID)
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

	policy := opt.(*loadbalancerv2.CreatePolicyRequest)
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
		pool, _ := m.GetPoolByID(ctx, lbID, policy.RedirectPoolID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return &entityv2.Policy{
		UUID: newPolicy.Policy.UUID,
	}, nil
}
func (m *MockProvider) ListPolicyOfListener(ctx context.Context, lbID, listenerID string) (*entityv2.ListPolicies, error) {
	policies := make([]*entityv2.Policy, 0)
	for _, p := range m.policies {
		if p.lbID == lbID && p.listenerID == listenerID {
			policies = append(policies, clone.Clone(p.Policy).(*entityv2.Policy))
		}
	}
	return &entityv2.ListPolicies{
		Items: policies,
	}, nil
}

//	func (m *MockProvider) GetPolicyByID(ctx context.Context, policyID string) (*objects.Policy, error) {
//		logger.Error("not implemented yet", "GetPolicyByID")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) UpdatePolicy(ctx context.Context, lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update policy %s of listener %s of load balancer %s", icon, policyID, listenerID, lbID)
	updateOpt := opt.(*loadbalancerv2.UpdatePolicyRequest)
	var policy *wrapPolicy
	for _, p := range m.policies {
		if p.lbID == lbID && p.listenerID == listenerID && p.UUID == policyID {
			policy = p
			break
		}
	}
	if policy == nil {
		return errs.ErrorNotFound
	}
	policy.Policy.RedirectPoolID = updateOpt.RedirectPoolID
	policy.Policy.Action = string(updateOpt.Action)
	policy.Policy.RedirectURL = updateOpt.RedirectURL
	policy.Policy.RedirectHTTPCode = updateOpt.RedirectHTTPCode
	policy.Policy.KeepQueryString = updateOpt.KeepQueryString
	newRules := make([]*entityv2.L7Rule, 0)
	for _, r := range updateOpt.Rules {
		newRules = append(newRules, &entityv2.L7Rule{
			UUID:               "rule-" + randID(),
			CompareType:        string(r.CompareType),
			RuleValue:          r.RuleValue,
			RuleType:           string(r.RuleType),
			ProvisioningStatus: consts.ACTIVE_LOADBALANCER_STATUS,
			OperatingStatus:    consts.ACTIVE_LOADBALANCER_STATUS,
		})
	}
	policy.Policy.L7Rules = newRules

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}
func (m *MockProvider) DeletePolicy(ctx context.Context, lbID, listenerID, policyID string) error {
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}

// --------------------------- Pool ---------------------------

//	func (m *MockProvider) GetPoolByName(ctx context.Context, lbID, name string) (*objects.Pool, error) {
//		logger.Error("not implemented yet", "GetPoolByName")
//		return nil, errs.ErrorNotImplemented
//	}
func (m *MockProvider) CreatePool(ctx context.Context, lbID string, opt loadbalancerv2.ICreatePoolRequest) (*entityv2.Pool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create pool of load balancer %s", icon, lbID)
	var (
		pool          *loadbalancerv2.CreatePoolRequest
		healthMonitor *loadbalancerv2.HealthMonitor
		member        []*loadbalancerv2.Member
	)
	pool = opt.ToRequestBody().(*loadbalancerv2.CreatePoolRequest)
	defaultPoolID := "pool-" + randID()

	lb, _ := m.GetLoadBalancerByID(ctx, lbID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return &entityv2.Pool{
		UUID: newPool.Pool.UUID,
	}, nil
}
func (m *MockProvider) ListPool(ctx context.Context, lbID string) (*entityv2.ListPools, error) {
	pools := make([]*entityv2.Pool, 0)
	for _, p := range m.pools {
		if p.lbID == lbID {
			pools = append(pools, clone.Clone(p.Pool).(*entityv2.Pool))
		}
	}
	return &entityv2.ListPools{
		Items: pools,
	}, nil
}
func (m *MockProvider) UpdatePoolMembers(ctx context.Context, lbID, poolID string, members loadbalancerv2.IUpdatePoolMembersRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update pool members %s of load balancer %s", icon, poolID, lbID)
	updateOpt := members.(*loadbalancerv2.UpdatePoolMembersRequest)
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
	newMembers := make([]*entityv2.Member, 0)
	for _, m := range updateOpt.Members {
		mem := m.(*loadbalancerv2.Member)
		newMembers = append(newMembers, &entityv2.Member{
			UUID:           "mem-" + randID(),
			Address:        mem.IpAddress,
			ProtocolPort:   mem.Port,
			Weight:         mem.Weight,
			MonitorPort:    mem.MonitorPort,
			SubnetID:       lbID,
			Name:           mem.Name,
			PoolID:         poolID,
			TypeCreate:     "????????",
			Backup:         false,
			DisplayStatus:  consts.ACTIVE_LOADBALANCER_STATUS,
			CreatedAt:      time.Now().Format(time.RFC3339),
			UpdatedAt:      time.Now().Format(time.RFC3339),
			CreatedBy:      "????????",
			ProgressStatus: consts.ACTIVE_LOADBALANCER_STATUS,
		})
	}
	pool.Members.Items = newMembers

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}

func (m *MockProvider) GetPoolByID(ctx context.Context, lbID, poolID string) (*entityv2.Pool, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return clone.Clone(p.Pool).(*entityv2.Pool), nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) GetPoolMembers(ctx context.Context, lbID, poolID string) (*entityv2.ListMembers, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return clone.Clone(p.Members).(*entityv2.ListMembers), nil
		}
	}
	return nil, errs.ErrorNotFound
}

func (m *MockProvider) DeletePool(ctx context.Context, lbID, poolID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete pool %s of load balancer %s", icon, poolID, lbID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}

func (m *MockProvider) UpdatePool(ctx context.Context, lbID, poolID string, opt loadbalancerv2.IUpdatePoolRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update pool %s of load balancer %s", icon, poolID, lbID)
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

	m.updatingStatus(lbID)
	go m.readyAfterTime(lbID)
	return nil
}

func (m *MockProvider) GetPoolHealthMonitorById(ctx context.Context, lbID, poolID string) (*entityv2.HealthMonitor, error) {
	for _, p := range m.pools {
		if p.lbID == lbID && p.GetId() == poolID {
			return clone.Clone(p.HealthMonitor).(*entityv2.HealthMonitor), nil
		}
	}
	return nil, errs.ErrorNotFound
}

// // --------------------------- Certificate ---------------------------

// func (m *MockProvider) ImportCertificate(ctx context.Context, opt *certificates.ImportOpts) (*objects.Certificate, error) {
// 	logger.Error("not implemented yet", "ImportCertificate")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) ListCertificates(ctx context.Context, ) ([]*objects.Certificate, error) {
// 	logger.Error("not implemented yet", "ListCertificates")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) GetCertificateByID(ctx context.Context, certID string) (*objects.Certificate, error) {
// 	logger.Error("not implemented yet", "GetCertificateByID")
// 	return nil, errs.ErrorNotImplemented
// }
// func (m *MockProvider) DeleteCertificate(ctx context.Context, certID string) error {
// 	logger.Error("not implemented yet", "DeleteCertificate")
// 	return errs.ErrorNotImplemented
// }
