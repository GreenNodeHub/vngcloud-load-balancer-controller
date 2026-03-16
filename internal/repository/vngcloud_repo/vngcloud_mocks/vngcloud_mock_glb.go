package vngcloud_mocks

import (
	"context"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	clone "github.com/huandu/go-clone"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

type wrapGlobalListener struct {
	*entityv2.GlobalListener
	lbID string
}
type wrapGlobalPool struct {
	*entityv2.GlobalPool
	lbID              string
	globalPoolMembers []*entityv2.GlobalPoolMember
}

// --------------------------- Global Load Balancer ---------------------------

func (m *MockProvider) ListGlobalPackages(ctx context.Context) (*entityv2.ListGlobalPackages, error) {
	return &entityv2.ListGlobalPackages{
		Items: []entityv2.GlobalPackage{
			{ID: "glb-pkg-001", Name: "glb-small"},
			{ID: "glb-pkg-002", Name: "glb-medium"},
		},
	}, nil
}

func (m *MockProvider) updatingGlobalStatus(lbID string) {
	logger := contexts.NewContext(context.TODO()).Log()
	var o *entityv2.GlobalLoadBalancer
	for _, lb := range m.glbs {
		if lb.ID == lbID {
			o = lb
			break
		}
	}
	if o == nil {
		logger.Error("Global Load Balancer not found")
		return
	}

	if m.WaitAfterTime == 0 {
		m.mu.Lock()
		o.UpdatedAt = time.Now().Format(time.RFC3339)
		o.Status = consts.ACTIVE_LOADBALANCER_STATUS
		m.mu.Unlock()
		return
	}

	m.mu.Lock()
	o.Status = consts.CREATED_LOADBALANCER_STATUS
	m.mu.Unlock()
}

func (m *MockProvider) readyGlobalAfterTime(lbID string) {
	if m.WaitAfterTime == 0 {
		return
	}
	logger := contexts.NewContext(context.TODO()).Log()
	var o *entityv2.GlobalLoadBalancer
	for _, lb := range m.glbs {
		if lb.ID == lbID {
			o = lb
			break
		}
	}
	if o == nil {
		logger.Error("Global Load Balancer not found")
		return
	}

	time.Sleep(m.WaitAfterTime)
	m.mu.Lock()
	o.UpdatedAt = time.Now().Format(time.RFC3339)
	o.Status = consts.ACTIVE_LOADBALANCER_STATUS
	m.mu.Unlock()
}

func (m *MockProvider) ListGlobalLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListGlobalLoadBalancers, error) {
	return &entityv2.ListGlobalLoadBalancers{
		Items: m.glbs,
	}, nil
}

func (m *MockProvider) GetGlobalLoadBalancerByID(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error) {
	for _, glb := range m.glbs {
		if glb.ID == glbID {
			return clone.Clone(glb).(*entityv2.GlobalLoadBalancer), nil
		}
	}
	logger := contexts.NewContext(ctx).Log()
	logger.Error("Global Load Balancer not found")
	return nil, domain.ErrorNotFound
}

func (m *MockProvider) GetGlobalLoadBalancerByName(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error) {
	for _, glb := range m.glbs {
		if glb.Name == glbID {
			return clone.Clone(glb).(*entityv2.GlobalLoadBalancer), nil
		}
	}
	return nil, nil
}

func (m *MockProvider) CreateGlobalLoadBalancer(ctx context.Context, glbOptions global.ICreateGlobalLoadBalancerRequest) (*entityv2.GlobalLoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create load balancer.", domain.RequestIcon)
	if glbOptions == nil {
		return nil, domain.ErrorInvalidInput
	}

	var lbOpt *global.CreateGlobalLoadBalancerRequest
	if opt, ok := glbOptions.(*global.CreateGlobalLoadBalancerRequest); ok {
		lbOpt = opt
	} else {
		return nil, domain.ErrorInvalidInput
	}

	lbID := "glb-" + randID()
	newLB := &entityv2.GlobalLoadBalancer{
		ID:      lbID,
		Name:    lbOpt.Name,
		Type:    string(lbOpt.Type),
		Package: lbOpt.Package,
		// UserId:      lbOpt.UserId,
		// Vips:        lbOpt.Vips,
		// Domains:     lbOpt.Domains,
		Description: "????????",
		CreatedAt:   time.Now().Format(time.RFC3339),
		UpdatedAt:   time.Now().Format(time.RFC3339),
		DeletedAt:   "",
		Status:      consts.ACTIVE_LOADBALANCER_STATUS,
	}

	m.mu.Lock()
	m.glbs = append(m.glbs, newLB)
	m.mu.Unlock()

	defaultPoolID := ""
	if lbOpt.GlobalPool != nil {
		pool, _ := m.CreateGlobalPool(ctx, lbID, lbOpt.GlobalPool.WithLoadBalancerId(newLB.ID))
		defaultPoolID = pool.ID
	}

	if lbOpt.GlobalListener != nil {
		m.CreateGlobalListener(ctx, lbID, lbOpt.GlobalListener.WithLoadBalancerId(newLB.ID).WithGlobalPoolId(defaultPoolID))
	}

	m.updatingGlobalStatus(newLB.ID)
	go m.readyGlobalAfterTime(newLB.ID)

	return &entityv2.GlobalLoadBalancer{
		ID: newLB.ID,
	}, nil
}

func (m *MockProvider) DeleteGlobalLoadBalancer(ctx context.Context, glbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete load balancer %s.", domain.RequestIcon, glbID)
	newLBs := make([]*entityv2.GlobalLoadBalancer, 0)
	for i := range m.glbs {
		if m.glbs[i].ID != glbID {
			newLBs = append(newLBs, m.glbs[i])
		}
	}
	if len(newLBs) == len(m.glbs) {
		logger.Error("Global Load Balancer not found")
		return domain.ErrorNotFound
	}

	m.glbs = newLBs
	return nil
}

func (m *MockProvider) WaitGlobalLoadBalancerActive(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Waiting for global load balancer %s to be ready", domain.WaitIcon, glbID)
	var resultLB *entityv2.GlobalLoadBalancer

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetGlobalLoadBalancerByID(ctx, glbID)
		if err != nil {
			logger.Errorf("Error getting global load balancer %s when wait active: %v", glbID, err)
			return false, err
		}
		if strings.ToUpper(lb.Status) == consts.ACTIVE_LOADBALANCER_STATUS {
			logger.Infof("%s Global load balancer %s is ready", domain.ReadyIcon, glbID)
			resultLB = lb
			return true, nil
		}

		logger.Infof("%s Global load balancer %s is `%s`, waiting...", domain.WaitIcon, glbID, lb.Status)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for the global loadbalancer %s with lb status %s", glbID, resultLB.Status)
	}

	return resultLB, err
}

func (m *MockProvider) ListGlobalPools(ctx context.Context, glbID string) (*entityv2.ListGlobalPools, error) {
	pools := make([]*entityv2.GlobalPool, 0)
	for _, p := range m.globalPools {
		if p.lbID == glbID {
			pools = append(pools, clone.Clone(p.GlobalPool).(*entityv2.GlobalPool))
		}
	}
	return &entityv2.ListGlobalPools{
		Items: pools,
	}, nil
}

func (m *MockProvider) CreateGlobalPool(ctx context.Context, glbID string, opt global.ICreateGlobalPoolRequest) (*entityv2.GlobalPool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create global pool of load balancer %s", domain.RequestIcon, glbID)
	pool := opt.ToRequestBody().(*global.CreateGlobalPoolRequest)
	newPool := &wrapGlobalPool{
		lbID: glbID,
		GlobalPool: &entityv2.GlobalPool{
			ID:                   "gpool-" + randID(),
			Name:                 pool.Name,
			Description:          "????????",
			Protocol:             string(pool.Protocol),
			Algorithm:            string(pool.Algorithm),
			Status:               consts.ACTIVE_LOADBALANCER_STATUS,
			CreatedAt:            time.Now().Format(time.RFC3339),
			UpdatedAt:            time.Now().Format(time.RFC3339),
			GlobalLoadBalancerID: glbID,
			StickySession:        nil,
			TLSEnabled:           nil,
			Health:               nil,
		},
	}

	poolMembers := make([]*entityv2.GlobalPoolMember, 0)
	for _, m := range pool.GlobalPoolMembers {
		poolMember := m.ToRequestBody().(*global.GlobalPoolMemberRequest)
		poolMemberID := "gpoolmem-" + randID()
		members := &entityv2.ListGlobalMembers{
			Items: make([]*entityv2.GlobalPoolMemberDetail, 0),
		}
		for _, mem := range poolMember.Members {
			member := mem.ToRequestBody().(*global.GlobalMemberRequest)
			members.Items = append(members.Items, &entityv2.GlobalPoolMemberDetail{
				ID:                   "gmem-" + randID(),
				CreatedAt:            time.Now().Format(time.RFC3339),
				UpdatedAt:            time.Now().Format(time.RFC3339),
				Name:                 member.Name,
				Description:          "????????",
				GlobalLoadBalancerID: glbID,
				Status:               consts.ACTIVE_LOADBALANCER_STATUS,
				GlobalPoolMemberID:   poolMemberID,
				SubnetID:             member.SubnetID,
				Address:              member.Address,
				Weight:               member.Weight,
				Port:                 member.Port,
				MonitorPort:          member.MonitorPort,
				BackupRole:           member.BackupRole,
			})
		}
		poolMembers = append(poolMembers, &entityv2.GlobalPoolMember{
			ID:                   poolMemberID,
			CreatedAt:            time.Now().Format(time.RFC3339),
			UpdatedAt:            time.Now().Format(time.RFC3339),
			Name:                 poolMember.Name,
			Description:          "????????",
			Region:               poolMember.Region,
			GlobalPoolID:         poolMember.PoolId,
			GlobalLoadBalancerID: glbID,
			TrafficDial:          poolMember.TrafficDial,
			VpcID:                poolMember.VPCID,
			Status:               consts.ACTIVE_LOADBALANCER_STATUS,
			Members:              members,
		})
	}
	newPool.globalPoolMembers = poolMembers

	health := pool.HealthMonitor.ToRequestBody().(*global.GlobalHealthMonitorRequest)
	newPool.Health = &entityv2.GlobalPoolHealthMonitor{
		ID:                   "ghealth-" + randID(),
		CreatedAt:            time.Now().Format(time.RFC3339),
		UpdatedAt:            time.Now().Format(time.RFC3339),
		DeletedAt:            nil,
		GlobalPoolID:         newPool.ID,
		GlobalLoadBalancerID: glbID,
		Protocol:             string(health.HealthCheckProtocol),
		Path:                 health.Path,
		Timeout:              health.Timeout,
		IntervalTime:         health.Interval,
		HealthyThreshold:     health.HealthyThreshold,
		UnhealthyThreshold:   health.UnhealthyThreshold,
		HTTPVersion:          (*string)(health.HttpVersion),
		HTTPMethod:           (*string)(health.HttpMethod),
		DomainName:           health.DomainName,
		SuccessCode:          health.SuccessCode,
		Status:               consts.ACTIVE_LOADBALANCER_STATUS,
	}

	m.mu.Lock()
	m.globalPools = append(m.globalPools, newPool)
	m.mu.Unlock()

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return &entityv2.GlobalPool{
		ID: newPool.GlobalPool.ID,
	}, nil
}

func (m *MockProvider) DeleteGlobalPool(ctx context.Context, glbID, poolID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete global pool %s of load balancer %s", domain.RequestIcon, poolID, glbID)
	isFound := false
	newPools := make([]*wrapGlobalPool, 0)
	for i, p := range m.globalPools {
		if p.lbID != glbID || p.ID != poolID {
			newPools = append(newPools, m.globalPools[i])
		} else {
			isFound = true
		}
	}
	if !isFound {
		logger.Error("Pool not found")
		return domain.ErrorNotFound
	}
	m.globalPools = newPools

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return nil
}

func (m *MockProvider) UpdateGlobalPool(ctx context.Context, glbID, poolID string, opt global.IUpdateGlobalPoolRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update global pool %s of load balancer %s", domain.RequestIcon, poolID, glbID)

	req := opt.ToRequestBody().(*global.UpdateGlobalPoolRequest)

	found := false
	m.mu.Lock()
	for _, p := range m.globalPools {
		if p.lbID == glbID && p.ID == poolID {
			p.Algorithm = string(req.Algorithm)
			if req.HealthMonitor != nil {
				health := req.HealthMonitor.ToRequestBody().(*global.GlobalHealthMonitorRequest)
				if p.Health != nil {
					p.Health.HealthyThreshold = health.HealthyThreshold
					p.Health.UnhealthyThreshold = health.UnhealthyThreshold
					p.Health.IntervalTime = health.Interval
					p.Health.Timeout = health.Timeout
					p.Health.Protocol = string(health.HealthCheckProtocol)
					p.Health.HTTPMethod = (*string)(health.HttpMethod)
					p.Health.HTTPVersion = (*string)(health.HttpVersion)
					p.Health.Path = health.Path
					p.Health.SuccessCode = health.SuccessCode
					p.Health.DomainName = health.DomainName
					p.Health.UpdatedAt = time.Now().Format(time.RFC3339)
				}
			}
			p.UpdatedAt = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}
	m.mu.Unlock()

	if !found {
		logger.Errorf("Global pool %s not found in load balancer %s", poolID, glbID)
		return domain.ErrorNotFound
	}

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return nil
}

func (m *MockProvider) ListGlobalPoolMembers(ctx context.Context, glbID, poolID string) (*entityv2.ListGlobalPoolMembers, error) {
	members := make([]*entityv2.GlobalPoolMember, 0)
	for _, p := range m.globalPools {
		if p.lbID == glbID && p.ID == poolID {
			for _, m := range p.globalPoolMembers {
				members = append(members, clone.Clone(m).(*entityv2.GlobalPoolMember))
			}
			break
		}
	}
	return &entityv2.ListGlobalPoolMembers{
		Items: members,
	}, nil
}

func (m *MockProvider) PatchGlobalPoolMembers(ctx context.Context, glbID, poolID string, opt global.IPatchGlobalPoolMembersRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request patch global pool member of load balancer %s", domain.RequestIcon, glbID)

	patch := opt.ToRequestBody().(*global.PatchGlobalPoolMembersRequest)
	for _, action := range patch.BulkActions {
		// action can be PatchGlobalPoolCreateBulkActionRequest or PatchGlobalPoolDeleteBulkActionRequest
		if rawAction, ok := action.(*global.PatchGlobalPoolCreateBulkActionRequest); ok {
			createPoolMemberOptions := rawAction.CreatePoolMember.ToRequestBody().(*global.GlobalPoolMemberRequest)
			poolMemberID := "gpoolmem-" + randID()
			members := &entityv2.ListGlobalMembers{
				Items: make([]*entityv2.GlobalPoolMemberDetail, 0),
			}
			for _, mem := range createPoolMemberOptions.Members {
				member := mem.ToRequestBody().(*global.GlobalMemberRequest)
				members.Items = append(members.Items, &entityv2.GlobalPoolMemberDetail{
					ID:                   "gmem-" + randID(),
					CreatedAt:            time.Now().Format(time.RFC3339),
					UpdatedAt:            time.Now().Format(time.RFC3339),
					Name:                 member.Name,
					Description:          "????????",
					GlobalLoadBalancerID: glbID,
					Status:               consts.ACTIVE_LOADBALANCER_STATUS,
					GlobalPoolMemberID:   poolMemberID,
					SubnetID:             member.SubnetID,
					Address:              member.Address,
					Weight:               member.Weight,
					Port:                 member.Port,
					MonitorPort:          member.MonitorPort,
					BackupRole:           member.BackupRole,
				})
			}
			newPoolMember := &entityv2.GlobalPoolMember{
				ID:                   poolMemberID,
				CreatedAt:            time.Now().Format(time.RFC3339),
				UpdatedAt:            time.Now().Format(time.RFC3339),
				Name:                 createPoolMemberOptions.Name,
				Description:          "????????",
				Region:               createPoolMemberOptions.Region,
				GlobalPoolID:         createPoolMemberOptions.PoolId,
				GlobalLoadBalancerID: glbID,
				TrafficDial:          createPoolMemberOptions.TrafficDial,
				VpcID:                createPoolMemberOptions.VPCID,
				Status:               consts.ACTIVE_LOADBALANCER_STATUS,
				Members:              members,
			}

			m.mu.Lock()
			for _, p := range m.globalPools {
				if p.lbID == glbID && p.ID == poolID {
					p.globalPoolMembers = append(p.globalPoolMembers, newPoolMember)
					break
				}
			}
			m.mu.Unlock()

			m.updatingGlobalStatus(glbID)
			go m.readyGlobalAfterTime(glbID)
		} else if rawAction, ok := action.(*global.PatchGlobalPoolDeleteBulkActionRequest); ok {
			poolMemberID := rawAction.ID
			isFound := false
			newPoolMembers := make([]*entityv2.GlobalPoolMember, 0)
			for _, p := range m.globalPools {
				if p.lbID == glbID && p.ID == poolID {
					for i, m := range p.globalPoolMembers {
						if m.ID != poolMemberID {
							newPoolMembers = append(newPoolMembers, p.globalPoolMembers[i])
						} else {
							isFound = true
						}
					}
					break
				}
			}
			if !isFound {
				logger.Error("Pool member not found")
				return domain.ErrorNotFound
			}
			m.mu.Lock()
			for _, p := range m.globalPools {
				if p.lbID == glbID && p.ID == poolID {
					p.globalPoolMembers = newPoolMembers
					break
				}
			}
			m.mu.Unlock()

			m.updatingGlobalStatus(glbID)
			go m.readyGlobalAfterTime(glbID)

		} else if rawAction, ok := action.(*global.PatchGlobalPoolUpdateBulkActionRequest); ok {
			updatePoolMemberOptions := rawAction.UpdatePoolMember.ToRequestBody().(*global.UpdateGlobalPoolMemberRequest)
			poolMemberID := rawAction.ID

			members := &entityv2.ListGlobalMembers{
				Items: make([]*entityv2.GlobalPoolMemberDetail, 0),
			}
			for _, mem := range updatePoolMemberOptions.Members {
				member := mem.ToRequestBody().(*global.GlobalMemberRequest)
				members.Items = append(members.Items, &entityv2.GlobalPoolMemberDetail{
					ID:                   "gmem-" + randID(),
					CreatedAt:            time.Now().Format(time.RFC3339),
					UpdatedAt:            time.Now().Format(time.RFC3339),
					Name:                 member.Name,
					Description:          "????????",
					GlobalLoadBalancerID: glbID,
					Status:               consts.ACTIVE_LOADBALANCER_STATUS,
					GlobalPoolMemberID:   poolMemberID,
					SubnetID:             member.SubnetID,
					Address:              member.Address,
					Weight:               member.Weight,
					Port:                 member.Port,
					MonitorPort:          member.MonitorPort,
					BackupRole:           member.BackupRole,
				})
			}

			isFound := false
			for _, p := range m.globalPools {
				if p.lbID == glbID && p.ID == poolID {
					for _, m := range p.globalPoolMembers {
						if m.ID == poolMemberID {
							m.TrafficDial = updatePoolMemberOptions.TrafficDial
							m.Members = members
							m.UpdatedAt = time.Now().Format(time.RFC3339)
							isFound = true
							break
						}
					}
					break
				}

			}
			if !isFound {
				logger.Error("Pool member not found")
				return domain.ErrorNotFound
			}
			m.updatingGlobalStatus(glbID)
			go m.readyGlobalAfterTime(glbID)

		} else {
			logger.Error("Invalid bulk action")
			return domain.ErrorInvalidInput
		}
	}

	return nil
}

func (m *MockProvider) ListGlobalListeners(ctx context.Context, glbID string) (*entityv2.ListGlobalListeners, error) {
	listeners := make([]*entityv2.GlobalListener, 0)
	for _, l := range m.globalListeners {
		if l.lbID == glbID {
			listeners = append(listeners, clone.Clone(l.GlobalListener).(*entityv2.GlobalListener))
		}
	}
	return &entityv2.ListGlobalListeners{
		Items: listeners,
	}, nil
}

func (m *MockProvider) GetGlobalListener(ctx context.Context, glbID, listenerID string) (*entityv2.GlobalListener, error) {
	logger := contexts.NewContext(ctx).Log()
	for _, l := range m.globalListeners {
		if l.lbID == glbID && l.ID == listenerID {
			return clone.Clone(l.GlobalListener).(*entityv2.GlobalListener), nil
		}
	}
	logger.Error("Global Listener not found")
	return nil, domain.ErrorNotFound
}

func (m *MockProvider) CreateGlobalListener(ctx context.Context, glbID string, opt global.ICreateGlobalListenerRequest) (*entityv2.GlobalListener, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create global listener of load balancer %s", domain.RequestIcon, glbID)
	listener := opt.ToRequestBody().(*global.CreateGlobalListenerRequest)
	newListener := &wrapGlobalListener{
		lbID: glbID,
		GlobalListener: &entityv2.GlobalListener{
			ID:                   "glis-" + randID(),
			Name:                 listener.Name,
			Description:          "????????",
			Protocol:             string(listener.Protocol),
			Port:                 listener.Port,
			Status:               consts.ACTIVE_LOADBALANCER_STATUS,
			CreatedAt:            time.Now().Format(time.RFC3339),
			UpdatedAt:            time.Now().Format(time.RFC3339),
			GlobalLoadBalancerID: glbID,
			GlobalPoolID:         listener.GlobalPoolId,
			TimeoutClient:        listener.TimeoutClient,
			TimeoutMember:        listener.TimeoutMember,
			TimeoutConnection:    listener.TimeoutConnection,
			AllowedCidrs:         listener.AllowedCidrs,
			Headers:              nil,
		},
	}

	m.mu.Lock()
	m.globalListeners = append(m.globalListeners, newListener)
	m.mu.Unlock()

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return &entityv2.GlobalListener{
		ID: newListener.GlobalListener.ID,
	}, nil

	// logger := contexts.NewContext(ctx).Log()
	// logger.Infof("%s Request create listener of load balancer %s", domain.RequestIcon, lbID)
	// listener := opt.ToRequestBody().(*loadbalancerv2.CreateListenerRequest)
	// newListener := &wrapListener{
	// 	lbID: lbID,
	// 	Listener: &entityv2.Listener{
	// 		UUID:                            "lis-" + randID(),
	// 		Name:                            listener.ListenerName,
	// 		Description:                     "????????",
	// 		Protocol:                        string(listener.ListenerProtocol),
	// 		ProtocolPort:                    listener.ListenerProtocolPort,
	// 		ConnectionLimit:                 0,
	// 		DefaultPoolId:                   "",
	// 		DefaultPoolName:                 "",
	// 		TimeoutClient:                   listener.TimeoutClient,
	// 		TimeoutMember:                   listener.TimeoutMember,
	// 		TimeoutConnection:               listener.TimeoutConnection,
	// 		AllowedCidrs:                    listener.AllowedCidrs,
	// 		DisplayStatus:                   consts.ACTIVE_LOADBALANCER_STATUS,
	// 		CreatedAt:                       time.Now().Format(time.RFC3339),
	// 		UpdatedAt:                       time.Now().Format(time.RFC3339),
	// 		ProgressStatus:                  consts.ACTIVE_LOADBALANCER_STATUS,
	// 		Headers:                         nil,
	// 		CertificateAuthorities:          nil,
	// 		DefaultCertificateAuthority:     nil,
	// 		ClientCertificateAuthentication: nil,
	// 	},
	// }
	// if listener.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS ||
	// 	listener.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTP {
	// 	if listener.Headers == nil {
	// 		return nil, errors.New("Missing Headers For HTTP/HTTPS Listener")
	// 	}
	// 	newListener.Listener.Headers = *listener.Headers
	// }
	// if listener.ListenerProtocol == loadbalancerv2.ListenerProtocolHTTPS {
	// 	if listener.DefaultCertificateAuthority == nil || *listener.DefaultCertificateAuthority == "" {
	// 		return nil, errors.New("Missing Default Certificate Authority For HTTPS Listener")
	// 	}
	// 	if listener.CertificateAuthorities == nil {
	// 		return nil, errors.New("Missing Certificate Authorities For HTTPS Listener")
	// 	}
	// 	// if listener.ClientCertificate == nil {
	// 	// 	return nil, errors.New("Missing Client Certificate For HTTPS Listener")
	// 	// }
	// 	newListener.Listener.CertificateAuthorities = *listener.CertificateAuthorities
	// 	newListener.Listener.DefaultCertificateAuthority = listener.DefaultCertificateAuthority
	// 	newListener.Listener.ClientCertificateAuthentication = listener.ClientCertificate
	// }
	// if listener.DefaultPoolId != nil && *listener.DefaultPoolId != "" {
	// 	newListener.DefaultPoolId = *listener.DefaultPoolId
	// 	pool, _ := m.GetPoolByID(ctx, lbID, newListener.DefaultPoolId)
	// 	if pool != nil {
	// 		newListener.DefaultPoolName = pool.Name
	// 	}
	// }

	// m.mu.Lock()
	// m.listeners = append(m.listeners, newListener)
	// m.mu.Unlock()

	// m.updatingStatus(lbID)
	// go m.readyAfterTime(lbID)
	// return &entityv2.Listener{
	// 	UUID: newListener.Listener.UUID,
	// }, nil
}

func (m *MockProvider) DeleteGlobalListener(ctx context.Context, glbID, listenerID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete global listener %s of load balancer %s", domain.RequestIcon, listenerID, glbID)
	isFound := false
	newListeners := make([]*wrapGlobalListener, 0)
	for i, l := range m.globalListeners {
		if l.lbID != glbID || l.ID != listenerID {
			newListeners = append(newListeners, m.globalListeners[i])
		} else {
			isFound = true
		}
	}
	if !isFound {
		logger.Error("Listener not found")
		return domain.ErrorNotFound
	}
	m.globalListeners = newListeners

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return nil
}

func (m *MockProvider) UpdateGlobalListener(ctx context.Context, glbID, listenerID string, opt global.IUpdateGlobalListenerRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update global listener %s of load balancer %s", domain.RequestIcon, listenerID, glbID)

	req := opt.ToRequestBody().(*global.UpdateGlobalListenerRequest)

	found := false
	m.mu.Lock()
	for _, l := range m.globalListeners {
		if l.lbID == glbID && l.ID == listenerID {
			l.AllowedCidrs = req.AllowedCidrs
			l.TimeoutClient = req.TimeoutClient
			l.TimeoutMember = req.TimeoutMember
			l.TimeoutConnection = req.TimeoutConnection
			l.GlobalPoolID = req.GlobalPoolId
			l.Headers = req.Headers
			l.UpdatedAt = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}
	m.mu.Unlock()

	if !found {
		logger.Errorf("Global listener %s not found in load balancer %s", listenerID, glbID)
		return domain.ErrorNotFound
	}

	m.updatingGlobalStatus(glbID)
	go m.readyGlobalAfterTime(glbID)
	return nil
}
