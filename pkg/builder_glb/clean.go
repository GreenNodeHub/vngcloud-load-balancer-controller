package builder

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/global"
)

type modelMember struct {
	name          string
	updateRequest global.IGlobalMemberRequest
}

func (m *modelMember) shouldDelete(activeClusterIDs []string) bool {
	for _, clusterID := range activeClusterIDs {
		if m.name == clusterID {
			return false
		}
	}
	return true
}

func (m *modelMember) print() {
	logrus.Infof("    m: %s", m.name)
}

func (m *modelMember) getIGlobalMemberRequest() global.IGlobalMemberRequest {
	return m.updateRequest
}

type modelPoolMember struct {
	id          string
	trafficDial int
	members     []*modelMember
}

func (m *modelPoolMember) print() {
	logrus.Infof("  pm: %s", m.id)
	for _, member := range m.members {
		member.print()
	}
}

func (m *modelPoolMember) shouldDelete(activeClusterIDs []string) bool {
	for _, member := range m.members {
		if !member.shouldDelete(activeClusterIDs) {
			return false
		}
	}
	return true
}

func (m *modelPoolMember) isNeedUpdate(activeClusterIDs []string) bool {
	for _, member := range m.members {
		if member.shouldDelete(activeClusterIDs) {
			return true
		}
	}
	return false
}

func (m *modelPoolMember) getDeleteAction() global.IBulkActionRequest {
	return global.NewPatchGlobalPoolDeleteBulkActionRequest(m.id)
}

func (m *modelPoolMember) getUpdateAction(activeClusterIDs []string) global.IBulkActionRequest {
	members := make([]global.IGlobalMemberRequest, 0)
	for _, member := range m.members {
		if !member.shouldDelete(activeClusterIDs) {
			members = append(members, member.getIGlobalMemberRequest())
		}
	}
	return global.NewPatchGlobalPoolUpdateBulkActionRequest(m.id,
		global.NewUpdateGlobalPoolMemberRequest(m.trafficDial).WithMembers(members...),
	)
}

type modelPool struct {
	id         string
	name       string
	poolMember []*modelPoolMember
}

func (m *modelPool) print() {
	logrus.Infof("p: %s - %s", m.id, m.name)
	for _, member := range m.poolMember {
		member.print()
	}
}

func (p *modelPool) shouldDelete(activeClusterIDs []string) bool {
	for _, member := range p.poolMember {
		if !member.shouldDelete(activeClusterIDs) {
			return false
		}
	}
	return true
}

type modelListener struct {
	id    string
	pools *modelPool
}

func (l *modelListener) print() {
	logrus.Infof("l: %s => %s", l.id, l.pools.id)
}

func (l *modelListener) shouldDelete(activeClusterIDs []string) bool {
	if l.pools == nil {
		return true
	}
	return l.pools.shouldDelete(activeClusterIDs)
}

type modelLB struct {
	listeners []*modelListener
	pools     []*modelPool
}

func (l *modelLB) print() {
	logrus.Infof("🔰 modelLB:")
	for _, listener := range l.listeners {
		listener.print()
	}
	for _, pool := range l.pools {
		pool.print()
	}
}

func (l *modelLB) getPoolByID(id string) *modelPool {
	for _, pool := range l.pools {
		if pool.id == id {
			return pool
		}
	}

	logrus.Errorf("🔰 Pool %s not found", id)
	return nil
}

func (l *vngcloudLBBuilder) CleanUp(ctx context.Context, activeClusterIDs []string) error {
	l.poolListenerHelper = poolListenerHelper{
		poolBuilders:     make([]*poolBuilderType, 0),
		listenerBuilders: make([]*ListenerBuilderType, 0),
	}
	err := l.build()
	if err != nil {
		return err
	}

	logger := contexts.NewContext(ctx).Log()

	logger.Info("🔰 Start cleaning up redundant resources")
	logger.Infof("🔰 Active clusters: %v", activeClusterIDs)

	// transfer data to model
	model := &modelLB{
		listeners: make([]*modelListener, 0),
		pools:     make([]*modelPool, 0),
	}
	logger.Infof("🔰 pool length = %d", len(l.GetPoolBuilders()))
	for _, pool := range l.GetPoolBuilders() {
		logger.Info("🔰 pool:", pool.id)
		_pool := &modelPool{
			id:         pool.GetID(),
			name:       pool.GetName(),
			poolMember: make([]*modelPoolMember, 0),
		}
		for _, poolMember := range pool.GlobalPoolMembers {
			_poolMember := &modelPoolMember{
				id:          poolMember.id,
				trafficDial: poolMember.TrafficDial,
				members:     make([]*modelMember, 0),
			}
			for _, member := range poolMember.Members {
				parser_member := member.(*global.GlobalMemberRequest)
				_member := &modelMember{
					name: parser_member.Name,
					updateRequest: global.NewGlobalMemberRequest(
						parser_member.Name,
						parser_member.Address,
						parser_member.SubnetID,
						parser_member.Port,
						parser_member.MonitorPort,
						parser_member.Weight,
						parser_member.BackupRole),
				}
				_poolMember.members = append(_poolMember.members, _member)
			}
			_pool.poolMember = append(_pool.poolMember, _poolMember)
		}
		model.pools = append(model.pools, _pool)
	}

	for _, listener := range l.GetListenerBuilders() {
		logger.Info("🔰 listener:", listener.id)
		logger.Info("🔰 getpool:", model.getPoolByID(listener.GlobalPoolId).id)
		_listener := &modelListener{
			pools: model.getPoolByID(listener.GlobalPoolId),
			id:    listener.GetID(),
		}
		model.listeners = append(model.listeners, _listener)
	}

	model.print()

	if _, err := l.provider.WaitGlobalLoadBalancerActive(ctx, l.GetLoadBalancerID()); err != nil {
		logger.Error("Failed to wait for loadbalancer active: ", err)
		return err
	}
	// clean up
	for _, listener := range model.listeners {
		if listener.shouldDelete(activeClusterIDs) {
			// remove listener ...
			if err := l.provider.DeleteGlobalListener(ctx, l.GetLoadBalancerID(), listener.id); err != nil {
				logger.Error("Failed to delete listener: ", err)
				return err
			}
			if _, err := l.provider.WaitGlobalLoadBalancerActive(ctx, l.GetLoadBalancerID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}

	for _, pool := range model.pools {
		if pool.shouldDelete(activeClusterIDs) {
			// remove pool ...
			if err := l.provider.DeleteGlobalPool(ctx, l.GetLoadBalancerID(), pool.id); err != nil {
				logger.Error("Failed to delete pool: ", err)
				return err
			}
			if _, err := l.provider.WaitGlobalLoadBalancerActive(ctx, l.GetLoadBalancerID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
			continue
		}

		bulkRequests := make([]global.IBulkActionRequest, 0)

		for _, poolMember := range pool.poolMember {
			if poolMember.shouldDelete(activeClusterIDs) {
				// add a delete action
				bulkRequests = append(bulkRequests, poolMember.getDeleteAction())
			} else if poolMember.isNeedUpdate(activeClusterIDs) {
				// add a update action
				bulkRequests = append(bulkRequests, poolMember.getUpdateAction(activeClusterIDs))
			}
		}

		if len(bulkRequests) == 0 {
			continue
		}

		patchOptions := global.NewPatchGlobalPoolMemberRequest(l.GetLoadBalancerID(), pool.id).
			WithBulkAction(bulkRequests...)

		if err := l.provider.PatchGlobalPoolMember(ctx, l.GetLoadBalancerID(), pool.id, patchOptions); err != nil {
			logger.Error("Failed to patch pool member: ", err)
			return err
		}
		if _, err := l.provider.WaitGlobalLoadBalancerActive(ctx, l.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}

	return nil
}
