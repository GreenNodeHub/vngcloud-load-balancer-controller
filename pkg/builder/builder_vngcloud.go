package builder

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

type LoadBalancerBuilder interface {
	BasicInfoHelper

	EnsurePool(pool *poolBuilderType, oldBuilder OldModelBuilder) error
	EnsureListener(listener *ListenerBuilderType, oldBuilder OldModelBuilder) error
	EnsureTags(tags map[string]string, oldBuilder OldModelBuilder) error

	//
	DeleteRedundantListeners(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error
	// delete redundant pools, should check if pool is used by other listeners or policy then ignore
	DeleteRedundantPools(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error

	CanDeleteWholeLoadBalancer(oldBuilder OldModelBuilder) bool

	CanDeleteWholeListener(oldListener OldListener) bool
}

// depend on loadBalancerID, get loadBalancer information by using provider, then build the ModelBuilder
func NewLoadBalancerBuilderByLoadBalancerID(
	ctx context.Context,
	loadBalancerID string,
	provider provider.Provider,
	annotationParser annotations.Parser,
	clusterID string,
) (LoadBalancerBuilder, error) {
	model := &vngcloudLBBuilder{
		provider:         provider,
		context:          ctx,
		logger:           contexts.NewContext(ctx).Log(),
		annotationParser: annotationParser,
		clusterID:        clusterID,
		basicInfoHelper: basicInfoHelper{
			loadBalancerID:   loadBalancerID,
			loadBalancerName: "",
			loadBalancerType: "",
			packageID:        "",
			scheme:           "",
			tags:             map[string]string{},
		},
		poolListenerHelper: poolListenerHelper{
			poolBuilders:     make([]*poolBuilderType, 0),
			listenerBuilders: make([]*ListenerBuilderType, 0),
		},
	}

	err := model.build()
	if err != nil {
		return nil, err
	}

	return model, nil
}

var _ LoadBalancerBuilder = &vngcloudLBBuilder{}

type vngcloudLBBuilder struct {
	basicInfoHelper

	poolListenerHelper

	logger           *logrus.Entry
	context          context.Context
	provider         provider.Provider
	annotationParser annotations.Parser
	clusterID        string
}

func (r *vngcloudLBBuilder) build() error {
	lb, err := r.provider.GetLoadBalancerByID(r.context, r.GetLoadBalancerID())
	if err != nil {
		return err
	}
	if lb == nil {
		return errs.ErrorLoadBalancerNotHaveInformation
	}

	r.loadBalancerID = lb.UUID
	r.loadBalancerName = lb.Name
	r.loadBalancerType = loadbalancerv2.LoadBalancerType(lb.Type)
	r.packageID = lb.PackageID
	r.scheme = loadbalancerv2.LoadBalancerScheme(lb.LoadBalancerSchema)

	// Get pools
	pools, err := r.provider.ListPool(r.context, r.loadBalancerID)
	if err != nil {
		return err
	}
	for _, pool := range pools.Items {
		poolBuilder, err := r.buildPool(pool, lb.Type == string(loadbalancerv2.LoadBalancerTypeLayer4))
		if err != nil {
			return err
		}
		r.poolBuilders = append(r.poolBuilders, poolBuilder)
	}

	// Get listeners
	listeners, err := r.provider.ListListenerOfLB(r.context, r.loadBalancerID)
	if err != nil {
		return err
	}
	for _, listener := range listeners.Items {
		listenerBuilder, err := r.buildListener(listener)
		if err != nil {
			return err
		}
		r.listenerBuilders = append(r.listenerBuilders, listenerBuilder)
	}

	// Get tags
	if err := r.buildTags(); err != nil {
		return err
	}
	return nil
}

func (r *vngcloudLBBuilder) buildPool(pool *entityv2.Pool, isL4 bool) (*poolBuilderType, error) {
	healthMonitor, err := r.provider.GetPoolHealthMonitorById(r.context, r.loadBalancerID, pool.UUID)
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

	members, err := r.provider.GetPoolMembers(r.context, r.loadBalancerID, pool.UUID)
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

func (r *vngcloudLBBuilder) buildListener(listener *entityv2.Listener) (*ListenerBuilderType, error) {
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
		IsL4:           r.loadBalancerType == loadbalancerv2.LoadBalancerTypeLayer4,
		ReferPoolName:  listener.DefaultPoolName,
	}

	// get policies
	policies, err := r.provider.ListPolicyOfListener(r.context, r.loadBalancerID, listener.UUID)
	if err != nil {
		return nil, err
	}
	for _, policy := range policies.Items {
		policyBuilder, err := r.buildPolicy(policy)
		if err != nil {
			return nil, err
		}
		listenerBuilder.policyBuilders = append(listenerBuilder.policyBuilders, policyBuilder)
	}
	return listenerBuilder, nil
}

func (r *vngcloudLBBuilder) buildPolicy(policy *entityv2.Policy) (*policyBuilderType, error) {
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

func (r *vngcloudLBBuilder) buildTags() error {
	tags, err := r.provider.ListTags(r.context, r.GetLoadBalancerID())
	if err != nil {
		return err
	}

	tagsMap := make(map[string]string)
	for _, tag := range tags.Items {
		if tag.SystemTag {
			r.logger.Warnf("Have system tag: %s, skip this tag.", tag.Key)
			continue
		}
		tagsMap[tag.Key] = tag.Value
	}

	r.tags = tagsMap
	return nil
}

// ----------------------------------------------------------------------------------------------------------------------------

func (r *vngcloudLBBuilder) EnsurePool(poolBuilder *poolBuilderType, oldBuilder OldModelBuilder) error {
	poolInPortal := r.GetPoolBuilderByName(poolBuilder.GetName())
	if poolInPortal == nil {
		if _pool, err := r.provider.CreatePool(r.context, r.GetLoadBalancerID(),
			poolBuilder.GetICreatePoolRequest(r.GetLoadBalancerID())); err != nil {
			r.logger.Error("Failed to create pool: ", err)
			return err
		} else {
			poolBuilder.SetID(_pool.UUID)
		}
		if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
			r.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
		// need to update to current builder, avoid mismatch data later
		r.AddClonePoolBuilder(poolBuilder)
	} else {
		poolBuilder.SetID(poolInPortal.GetID())
		updateOptions, message := VNGHelper.ComparePoolBuilder(r.GetLoadBalancerID(), poolInPortal, poolBuilder)
		if updateOptions != nil {
			r.logger.Info("Need update pool: ", strings.Join(message, ", "))
			err := r.provider.UpdatePool(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
				updateOptions.WithLoadBalancerId(r.GetLoadBalancerID()))
			if err != nil {
				r.logger.Error("Failed to update pool: ", err)
				return err
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}

		// ensure pool members, with default pool, should merge pool members, otherwise, should update
		if poolBuilder.GetName() == consts.DEFAULT_NAME_DEFAULT_POOL {
			updateOptions, err := VNGHelper.MergePoolMembers(r.GetLoadBalancerID(),
				oldBuilder,
				poolInPortal,
				poolBuilder)
			if err != nil {
				r.logger.Error("Failed to merge pool members: ", err)
				return err
			}
			if updateOptions == nil {
				return nil
			}
			err = r.provider.UpdatePoolMembers(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
				updateOptions)
			if err != nil {
				r.logger.Error("Failed to update pool members: ", err)
				return err
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		} else // normal pool
		if !VNGHelper.ComparePoolMembers(poolInPortal.Members, poolBuilder.Members, true) {
			err := r.provider.UpdatePoolMembers(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
				poolBuilder.GetIUpdatePoolMembersRequest(r.GetLoadBalancerID()))
			if err != nil {
				r.logger.Error("Failed to update pool members: ", err)
				return err
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}
	return nil
}

func (r *vngcloudLBBuilder) EnsureListener(listenerBuilder *ListenerBuilderType, oldBuilder OldModelBuilder) error {
	// listenerInPortal := currentBuilder.GetListenerBuilderByName(listenerBuilder.GetName())
	listenerInPortal := r.GetListenerBuilderByPort(listenerBuilder.ListenerProtocolPort)
	if listenerInPortal == nil {
		if _lis, err := r.provider.CreateListener(r.context, r.GetLoadBalancerID(),
			listenerBuilder.GetICreateListenerRequest().WithLoadBalancerId(r.GetLoadBalancerID()),
		); err != nil {
			r.logger.Error("Failed to create listener: ", err)
			return err
		} else {
			listenerBuilder.SetID(_lis.UUID)
		}
		if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
			r.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
		// need to update to current builder, avoid mismatch data later
		r.AddCloneListenerBuilder(listenerBuilder)
		listenerInPortal = r.GetListenerBuilderByPort(listenerBuilder.ListenerProtocolPort)
	} else {
		listenerBuilder.SetID(listenerInPortal.GetID())

		// if mismatch listener protocol, return error => user must delete listener in portal ..........................
		if listenerInPortal.ListenerProtocol != listenerBuilder.ListenerProtocol {
			r.logger.Error("Listener protocol mismatch: ", listenerInPortal.ListenerProtocol, listenerBuilder.ListenerProtocol)
			return nil
		}

		updateOptions, message := VNGHelper.CompareListenerBuilder(r.GetLoadBalancerID(), listenerInPortal, listenerBuilder)
		if updateOptions != nil {
			r.logger.Info("Need update listener: ", strings.Join(message, ", "))
			err := r.provider.UpdateListener(r.context, r.GetLoadBalancerID(), listenerInPortal.GetID(), updateOptions)
			if err != nil {
				r.logger.Error("Failed to update listener: ", err)
				return err
			}

			// need to update to current builder, avoid mismatch data later
			listenerInPortal.DefaultPoolId = &updateOptions.DefaultPoolId
			listenerInPortal.ReferPoolName = ""
			if p := r.GetPoolBuilderByID(updateOptions.DefaultPoolId); p != nil { // in ensurePool, should update to the latest infomation
				listenerInPortal.ReferPoolName = p.GetName()
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}

	// ensure policy
	for _, policyBuilder := range listenerBuilder.GetPolicyBuilders() {
		// set policy redirect to pool id if exists
		if policyBuilder.ReferPoolName != "" {
			referPool := r.GetPoolBuilderByName(policyBuilder.ReferPoolName)
			if referPool == nil {
				r.logger.Error("Failed to get refer pool: ", policyBuilder.ReferPoolName)
				return nil
			}
			policyBuilder.RedirectPoolID = referPool.GetID()
		}

		policyInPortal := listenerInPortal.GetPolicyBuilderByName(policyBuilder.GetName())
		if policyInPortal == nil {
			if _, err := r.provider.CreatePolicy(r.context, r.GetLoadBalancerID(), listenerInPortal.GetID(),
				policyBuilder.GetICreatePolicyRequest(r.GetLoadBalancerID(), listenerInPortal.GetID())); err != nil {
				r.logger.Error("Failed to create policy: ", err)
				return err
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		} else {
			policyBuilder.SetID(policyInPortal.GetID())
			updateOptions, message := VNGHelper.ComparePolicyBuilder(r.GetLoadBalancerID(), listenerBuilder.GetID(),
				policyInPortal, policyBuilder)
			if updateOptions != nil {
				r.logger.Info("Need update policy: ", strings.Join(message, ", "))
				err := r.provider.UpdatePolicy(r.context, r.GetLoadBalancerID(), listenerBuilder.GetID(), policyInPortal.GetID(), updateOptions)
				if err != nil {
					r.logger.Error("Failed to update policy: ", err)
					return err
				}

				// need to update to current builder, avoid mismatch data later
				policyInPortal.ReferPoolName = ""
				policyInPortal.RedirectPoolID = updateOptions.RedirectPoolID
				if policyInPortal.RedirectPoolID != "" {
					if p := r.GetPoolBuilderByID(updateOptions.RedirectPoolID); p != nil {
						policyInPortal.ReferPoolName = p.GetName()
					}
				}
				if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
					r.logger.Error("Failed to wait for loadbalancer active: ", err)
					return err
				}
			}
		}
	}
	return nil
}

func (r *vngcloudLBBuilder) DeleteRedundantListeners(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error {
	// delete redundant listeners
	for _, oldListener := range oldBuilder.GetOldListeners() {
		currentListener := r.GetListenerBuilderByName(oldListener.GetName())
		newListener := newBuilder.GetListenerBuilderByName(oldListener.GetName())
		if currentListener == nil || currentListener.IsDeleted() || newListener != nil {
			continue
		}

		// delete whole listener
		if r.CanDeleteWholeListener(oldListener) {
			if err := r.provider.DeleteListener(r.context, r.GetLoadBalancerID(), oldListener.GetID()); err != nil {
				r.logger.Error("Failed to delete listener: ", err)
				return err
			}
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
			currentListener.SetIsDeleted(true)
			continue
		}

		// delete redundant policy
		for _, policy := range oldListener.GetOldPolicies() {
			currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName())
			newPolicy := newListener.GetPolicyBuilderByName(policy.GetName())

			if currentPolicy == nil || currentPolicy.IsDeleted() || newPolicy != nil {
				continue
			}

			// delete whole policy
			if err := r.provider.DeletePolicy(r.context, r.GetLoadBalancerID(), currentListener.GetID(), currentPolicy.GetID()); err != nil {
				r.logger.Error("Failed to delete policy: ", err)
				return err
			}
			currentPolicy.SetIsDeleted(true)
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}

		}
	}
	return nil
}

// delete redundant pools, should check if pool is used by other listeners or policy then ignore
func (r *vngcloudLBBuilder) DeleteRedundantPools(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error {
	for _, pool := range oldBuilder.GetOldPools() {
		if currentPool := r.GetPoolBuilderByName(pool.GetName()); currentPool != nil &&
			newBuilder.GetPoolBuilderByName(pool.GetName()) == nil {
			if currentPool.IsDeleted() {
				continue
			}
			if r.IsPoolInUseByOtherListener(currentPool.GetID()) {
				r.logger.Infof("pool \"%s\" is used by other listeners, ignore delete.", pool.GetName())
				continue
			}
			if err := r.provider.DeletePool(r.context, r.GetLoadBalancerID(), pool.GetID()); err != nil {
				r.logger.Error("Failed to delete pool: ", err)
				return err
			}
			currentPool.SetIsDeleted(true)
			if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}
	return nil
}

func (r *vngcloudLBBuilder) CanDeleteWholeLoadBalancer(oldBuilder OldModelBuilder) bool {
	if len(oldBuilder.GetOldListeners()) < len(r.GetListenerBuilders()) {
		r.logger.Debugf("Can't delete whole loadbalancer, len(oldListeners) < len(currentListeners) (%d < %d)",
			len(oldBuilder.GetOldListeners()), len(r.GetListenerBuilders()))
		return false
	}
	if len(oldBuilder.GetOldPools()) < len(r.GetPoolBuilders()) {
		r.logger.Debugf("Can't delete whole loadbalancer, len(oldPools) < len(currentPools) (%d < %d)",
			len(oldBuilder.GetOldPools()), len(r.GetPoolBuilders()))
		return false
	}
	// if listener not exists, return false
	for _, listener := range oldBuilder.GetOldListeners() {
		currentListener := r.GetListenerBuilderByName(listener.GetName())
		if currentListener == nil {
			r.logger.Debugf("Can't delete whole loadbalancer, listener not exists: %s", listener.GetName())
			return false
		}
		// if policy not exists, return false
		currentPolicies := currentListener.GetPolicyBuilders()
		oldPolicies := listener.GetOldPolicies()
		if len(oldPolicies) < len(currentPolicies) {
			r.logger.Debugf("Can't delete whole loadbalancer, len(oldPolicies) < len(currentPolicies) (%d < %d)",
				len(oldPolicies), len(currentPolicies))
			return false
		}
		for _, policy := range oldPolicies {
			if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy == nil {
				r.logger.Debugf("Can't delete whole loadbalancer, policy not exists: %s", policy.GetName())
				return false
			}
		}
	}
	// if pool not exists, return false
	for _, pool := range oldBuilder.GetOldPools() {
		if currentPool := r.GetPoolBuilderByName(pool.GetName()); currentPool == nil {
			r.logger.Debugf("Can't delete whole loadbalancer, pool not exists: %s", pool.GetName())
			return false
		}
	}
	r.logger.Debug("Can delete whole loadbalancer")
	return true
}

func (r *vngcloudLBBuilder) CanDeleteWholeListener(oldListener OldListener) bool {
	currentListener := r.GetListenerBuilderByName(oldListener.GetName())
	if len(oldListener.GetOldPolicies()) < len(currentListener.GetPolicyBuilders()) {
		r.logger.Debugf("Can't delete whole listener, len(oldPolicies) < len(currentPolicies) (%d < %d)",
			len(oldListener.GetOldPolicies()), len(currentListener.GetPolicyBuilders()))
		return false
	}
	for _, currentPolicy := range currentListener.GetPolicyBuilders() {
		if oldPolicy := oldListener.GetOldPolicyByName(currentPolicy.GetName()); oldPolicy == nil {
			r.logger.Debugf("Can't delete whole listener, policy not exists: %s", currentPolicy.GetName())
			return false
		}
	}

	r.logger.Debugf("Can delete whole listener %s.", oldListener.GetName())
	return true
}

func (r *vngcloudLBBuilder) DeleteListener(id string) error {
	currentListener := r.GetListenerBuilderByID(id)
	if err := r.provider.DeleteListener(r.context, r.GetLoadBalancerID(), id); err != nil {
		r.logger.Error("Failed to delete listener: ", err)
		return err
	}
	if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
		r.logger.Error("Failed to wait for loadbalancer active: ", err)
		return err
	}
	currentListener.SetIsDeleted(true)
	return nil
}

// ----------------------------------------------------------------------------------------------------------------------------

// after create pool, use this function to update current builder
func (l *vngcloudLBBuilder) AddClonePoolBuilder(pool *poolBuilderType) {
	clone := &poolBuilderType{
		commonBuilder: commonBuilder{
			name: pool.GetName(),
			id:   pool.GetID(),
		},
		Algorithm:     pool.Algorithm,
		PoolProtocol:  pool.PoolProtocol,
		Stickiness:    pool.Stickiness,
		TLSEncryption: pool.TLSEncryption,
		HealthMonitor: pool.HealthMonitor,
		IsL4:          pool.IsL4,
		Members:       make([]*loadbalancerv2.Member, 0),
		isDeleted:     false,
	}
	l.AddPoolBuilder(clone)
}

// after create listener, use this function to update current builder
func (l *vngcloudLBBuilder) AddCloneListenerBuilder(listener *ListenerBuilderType) {
	clone := &ListenerBuilderType{
		commonBuilder: commonBuilder{
			name: listener.GetName(),
			id:   listener.GetID(),
		},
		isDeleted:      false,
		policyBuilders: make([]*policyBuilderType, 0),
		CreateListenerRequest: loadbalancerv2.CreateListenerRequest{
			AllowedCidrs:                listener.CreateListenerRequest.AllowedCidrs,
			ListenerName:                listener.CreateListenerRequest.ListenerName,
			ListenerProtocol:            listener.CreateListenerRequest.ListenerProtocol,
			ListenerProtocolPort:        listener.CreateListenerRequest.ListenerProtocolPort,
			TimeoutClient:               listener.CreateListenerRequest.TimeoutClient,
			TimeoutConnection:           listener.CreateListenerRequest.TimeoutConnection,
			TimeoutMember:               listener.CreateListenerRequest.TimeoutMember,
			DefaultPoolId:               listener.CreateListenerRequest.DefaultPoolId,
			CertificateAuthorities:      listener.CreateListenerRequest.CertificateAuthorities,
			ClientCertificate:           listener.CreateListenerRequest.ClientCertificate,
			DefaultCertificateAuthority: listener.CreateListenerRequest.DefaultCertificateAuthority,
		},
		ReferPoolName: listener.ReferPoolName,
		IsL4:          listener.IsL4,
	}
	l.AddListenerBuilder(clone)
}
