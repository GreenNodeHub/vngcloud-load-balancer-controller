package builder

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

// depend on loadBalancerID, get loadBalancer information by using provider, then build the LoadbalancerBuilder
func NewLoadBalancerBuilderByLoadBalancerID(
	ctx context.Context,
	loadBalancerID string,
	provider provider.Provider,
) (LoadbalancerBuilder, error) {
	model := &vngcloudLBBuilder{
		lbBuilder: lbBuilder{
			loadBalancerID: loadBalancerID,
			logger:         contexts.NewContext(ctx).Log(),
			poolBuilders:   make([]*poolBuilderType, 0),
		},
		provider: provider,
	}

	err := model.build()
	if err != nil {
		return nil, err
	}

	return model, nil
}

type vngcloudLBBuilder struct {
	provider provider.Provider
	lbBuilder
}

func (b *vngcloudLBBuilder) build() error {
	lb, err := b.provider.GetLoadBalancerByID(b.loadBalancerID)
	if err != nil {
		return err
	}
	if lb == nil {
		return errs.ErrorLoadBalancerNotHaveInformation
	}

	b.loadBalancerID = lb.UUID
	b.loadBalancerName = lb.Name
	b.LoadBalancerType = loadbalancerv2.LoadBalancerType(lb.Type)
	b.packageID = lb.PackageID
	b.Scheme = loadbalancerv2.LoadBalancerScheme(lb.LoadBalancerSchema)

	// Get pools
	pools, err := b.provider.ListPool(b.loadBalancerID)
	if err != nil {
		return err
	}
	for _, pool := range pools.Items {
		poolBuilder, err := b.buildPool(pool, lb.Type == string(loadbalancerv2.LoadBalancerTypeLayer4))
		if err != nil {
			return err
		}
		b.poolBuilders = append(b.poolBuilders, poolBuilder)
	}

	// Get listeners
	listeners, err := b.provider.ListListenerOfLB(b.loadBalancerID)
	if err != nil {
		return err
	}
	for _, listener := range listeners.Items {
		listenerBuilder, err := b.buildListener(listener)
		if err != nil {
			return err
		}
		b.listenerBuilders = append(b.listenerBuilders, listenerBuilder)
	}
	return nil
}

func (b *vngcloudLBBuilder) buildPool(pool *entityv2.Pool, isL4 bool) (*poolBuilderType, error) {
	healthMonitor, err := b.provider.GetPoolHealthMonitorById(b.loadBalancerID, pool.UUID)
	if err != nil {
		return nil, err
	}
	poolBuilder := &poolBuilderType{
		commonBuilder: commonBuilder{
			Name: pool.Name,
			ID:   pool.UUID,
		},
		Algorithm:     loadbalancerv2.PoolAlgorithm(pool.LoadBalanceMethod),
		PoolName:      pool.Name,
		PoolProtocol:  loadbalancerv2.PoolProtocol(pool.Protocol),
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: &loadbalancerv2.HealthMonitor{
			HealthyThreshold:    healthMonitor.HealthyThreshold,
			UnhealthyThreshold:  healthMonitor.UnhealthyThreshold,
			Interval:            healthMonitor.Interval,
			Timeout:             healthMonitor.Timeout,
			HealthCheckPath:     healthMonitor.HealthCheckPath,
			SuccessCode:         healthMonitor.SuccessCode,
			HealthCheckProtocol: loadbalancerv2.HealthCheckProtocol(healthMonitor.HealthCheckProtocol),
			HealthCheckMethod:   (*loadbalancerv2.HealthCheckMethod)(healthMonitor.HealthCheckMethod),
			HttpVersion:         (*loadbalancerv2.HealthCheckHttpVersion)(healthMonitor.HttpVersion),
			DomainName:          healthMonitor.DomainName,
		},
		IsL4:    isL4,
		Members: make([]*loadbalancerv2.Member, 0),
	}
	if !isL4 {
		poolBuilder.Stickiness = &pool.Stickiness
		poolBuilder.TLSEncryption = &pool.TLSEncryption
	}

	members, err := b.provider.GetPoolMembers(b.loadBalancerID, pool.UUID)
	if err != nil {
		return nil, err
	}

	for _, member := range members.Items {
		poolBuilder.Members = append(poolBuilder.Members, &loadbalancerv2.Member{
			Name:        member.Name,
			IpAddress:   member.Address,
			Port:        member.ProtocolPort,
			MonitorPort: member.MonitorPort,
			Backup:      member.Backup,
			Weight:      member.Weight,
		})
	}
	return poolBuilder, nil
}

func (b *vngcloudLBBuilder) buildListener(listener *entityv2.Listener) (*listenerBuilderType, error) {
	listenerBuilder := &listenerBuilderType{
		commonBuilder: commonBuilder{
			Name: listener.Name,
			ID:   listener.UUID,
		},
		CreateListenerRequest: loadbalancerv2.CreateListenerRequest{
			AllowedCidrs:                listener.AllowedCidrs,
			ListenerName:                listener.Name,
			ListenerProtocol:            loadbalancerv2.ListenerProtocol(listener.Protocol),
			ListenerProtocolPort:        listener.ProtocolPort,
			TimeoutClient:               listener.TimeoutClient,
			TimeoutMember:               listener.TimeoutMember,
			TimeoutConnection:           listener.TimeoutConnection,
			DefaultPoolId:               &listener.DefaultPoolId,
			CertificateAuthorities:      &listener.CertificateAuthorities,
			ClientCertificate:           listener.ClientCertificateAuthentication,
			DefaultCertificateAuthority: listener.DefaultCertificateAuthority,
		},
	}
	return listenerBuilder, nil
}
