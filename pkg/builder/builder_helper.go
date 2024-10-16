package builder

import (
	"github.com/sirupsen/logrus"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
)

type PoolListenerHelper interface {
	AddPoolBuilder(pool *poolBuilderType)
	GetPoolBuilders() []*poolBuilderType
	GetPoolBuilderByName(name string) *poolBuilderType
	GetPoolBuilderByID(id string) *poolBuilderType

	IsPoolInUseByOtherListener(poolID string) bool

	AddListenerBuilder(listener *ListenerBuilderType)
	GetListenerBuilders() []*ListenerBuilderType
	GetListenerBuilderByName(name string) *ListenerBuilderType
	GetListenerBuilderByPort(port int) *ListenerBuilderType
	GetListenerBuilderByID(id string) *ListenerBuilderType
}

var _ PoolListenerHelper = &poolListenerHelper{}

type poolListenerHelper struct {
	poolBuilders     []*poolBuilderType
	listenerBuilders []*ListenerBuilderType
}

func (l *poolListenerHelper) AddPoolBuilder(pool *poolBuilderType) {
	for _, p := range l.poolBuilders {
		if p.GetName() == pool.GetName() {
			return
		}
	}
	l.poolBuilders = append(l.poolBuilders, pool)
}

func (l *poolListenerHelper) GetPoolBuilders() []*poolBuilderType {
	return l.poolBuilders
}

func (l *poolListenerHelper) GetPoolBuilderByName(name string) *poolBuilderType {
	for _, pool := range l.poolBuilders {
		if pool.GetName() == name {
			return pool
		}
	}
	return nil
}

func (l *poolListenerHelper) GetPoolBuilderByID(id string) *poolBuilderType {
	for _, pool := range l.poolBuilders {
		if pool.GetID() == id {
			return pool
		}
	}
	return nil
}

func (l *poolListenerHelper) AddListenerBuilder(listener *ListenerBuilderType) {
	for _, l := range l.listenerBuilders {
		if l.GetName() == listener.GetName() {
			return
		}
	}
	l.listenerBuilders = append(l.listenerBuilders, listener)
}

func (l *poolListenerHelper) GetListenerBuilders() []*ListenerBuilderType {
	return l.listenerBuilders
}

func (l *poolListenerHelper) GetListenerBuilderByName(name string) *ListenerBuilderType {
	for _, listener := range l.listenerBuilders {
		if listener.GetName() == name {
			return listener
		}
	}
	return nil
}

func (l *poolListenerHelper) GetListenerBuilderByPort(port int) *ListenerBuilderType {
	for _, listener := range l.listenerBuilders {
		if listener.ListenerProtocolPort == port {
			return listener
		}
	}
	return nil
}

func (l *poolListenerHelper) GetListenerBuilderByID(id string) *ListenerBuilderType {
	for _, listener := range l.listenerBuilders {
		if listener.GetID() == id {
			return listener
		}
	}
	return nil
}

func (l *poolListenerHelper) IsPoolInUseByOtherListener(poolID string) bool {
	// check if the pool is used by other listener
	for _, listener := range l.listenerBuilders {
		if !listener.IsDeleted() && *listener.DefaultPoolId == poolID {
			logrus.Debugf("Pool %s is used by listener %s.", poolID, listener.GetName())
			return true
		}

		if listener.IsDeleted() {
			continue
		}

		// check if the pool is used by policy
		for _, policy := range listener.GetPolicyBuilders() {
			if !policy.IsDeleted() && policy.RedirectPoolID == poolID {
				logrus.Debugf("Pool %s is used by policy %s.", poolID, policy.GetName())
				return true
			}
		}
	}
	return false
}

// ------------------------------------------------------------

type BasicInfoHelper interface {
	GetLoadBalancerID() string
	// return the real name in portal or the name that user specified in the annotation
	GetLoadBalancerName() string
	GetPackageID() string
	GetScheme() loadbalancerv2.LoadBalancerScheme
	GetLoadBalancerType() loadbalancerv2.LoadBalancerType
	GetTags() map[string]string
}

var _ BasicInfoHelper = &basicInfoHelper{}

type basicInfoHelper struct {
	loadBalancerID   string
	loadBalancerName string
	loadBalancerType loadbalancerv2.LoadBalancerType
	packageID        string
	scheme           loadbalancerv2.LoadBalancerScheme
	tags             map[string]string
}

func (l *basicInfoHelper) GetLoadBalancerID() string {
	return l.loadBalancerID
}

// return the real name in portal or the name that user specified in the annotation
func (l *basicInfoHelper) GetLoadBalancerName() string {
	return l.loadBalancerName
}

func (l *basicInfoHelper) GetPackageID() string {
	return l.packageID
}

func (l *basicInfoHelper) GetScheme() loadbalancerv2.LoadBalancerScheme {
	return l.scheme
}

func (l *basicInfoHelper) GetLoadBalancerType() loadbalancerv2.LoadBalancerType {
	return l.loadBalancerType
}

func (l *basicInfoHelper) GetTags() map[string]string {
	return l.tags
}
