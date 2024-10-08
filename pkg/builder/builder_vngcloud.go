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
			name: pool.Name,
			id:   pool.UUID,
		},
		Algorithm:     loadbalancerv2.PoolAlgorithm(pool.LoadBalanceMethod),
		PoolProtocol:  loadbalancerv2.PoolProtocol(pool.Protocol),
		Stickiness:    pool.Stickiness,
		TLSEncryption: pool.TLSEncryption,
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

func (b *vngcloudLBBuilder) buildListener(listener *entityv2.Listener) (*ListenerBuilderType, error) {
	listenerBuilder := &ListenerBuilderType{
		commonBuilder: commonBuilder{
			name: listener.Name,
			id:   listener.UUID,
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
		isDeleted:      false,
		policyBuilders: make([]*policyBuilderType, 0),
		IsL4:           b.LoadBalancerType == loadbalancerv2.LoadBalancerTypeLayer4,
		ReferPoolName:  listener.DefaultPoolName,
	}

	// get policies
	policies, err := b.provider.ListPolicyOfListener(b.loadBalancerID, listener.UUID)
	if err != nil {
		return nil, err
	}
	for _, policy := range policies.Items {
		policyBuilder, err := b.buildPolicy(policy)
		if err != nil {
			return nil, err
		}
		listenerBuilder.policyBuilders = append(listenerBuilder.policyBuilders, policyBuilder)
	}
	return listenerBuilder, nil
}

func (b *vngcloudLBBuilder) buildPolicy(policy *entityv2.Policy) (*policyBuilderType, error) {
	policyBuilder := &policyBuilderType{
		commonBuilder: commonBuilder{
			name: policy.Name,
			id:   policy.UUID,
		},
		isDeleted:        false,
		Action:           loadbalancerv2.PolicyAction(policy.Action),
		Rules:            nil,
		RedirectPoolID:   policy.RedirectPoolID,
		RedirectURL:      policy.RedirectURL,
		RedirectHTTPCode: policy.RedirectHTTPCode,
		KeepQueryString:  policy.KeepQueryString,
		ReferPoolName:    policy.RedirectPoolName,
	}
	rules := make([]loadbalancerv2.L7RuleRequest, 0)
	for _, rule := range policy.L7Rules {
		rules = append(rules, loadbalancerv2.L7RuleRequest{
			CompareType: loadbalancerv2.PolicyCompareType(rule.CompareType),
			RuleType:    loadbalancerv2.PolicyRuleType(rule.RuleType),
			RuleValue:   rule.RuleValue,
		})
	}
	policyBuilder.Rules = rules
	return policyBuilder, nil
}
