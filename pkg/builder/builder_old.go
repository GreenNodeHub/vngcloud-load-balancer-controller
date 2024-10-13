package builder

import (
	"strings"

	"github.com/sirupsen/logrus"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

type OldPool interface {
	GetID() string
	GetName() string
}

var _ OldPool = &oldPool{}

type oldPool struct {
	commonBuilder
}

// ------------------------------------------------------------

type OldPolicy interface {
	// GetID() string
	GetName() string
}

var _ OldPolicy = &oldPolicy{}

type oldPolicy struct {
	commonBuilder
}

// ------------------------------------------------------------

type OldListener interface {
	GetOldPolicies() []OldPolicy
	GetOldPolicyByName(name string) OldPolicy
	GetID() string
	GetName() string
}

var _ OldListener = &oldListener{}

type oldListener struct {
	oldPolicies []OldPolicy
	commonBuilder
}

func (l *oldListener) GetOldPolicies() []OldPolicy {
	return l.oldPolicies
}

func (l *oldListener) GetOldPolicyByName(name string) OldPolicy {
	for _, policy := range l.oldPolicies {
		if policy.GetName() == name {
			return policy
		}
	}
	return nil
}

// ------------------------------------------------------------

type OldModelBuilder interface {
	GetOldListeners() []OldListener
	GetOldPools() []OldPool
	GetDefaultPoolMembers() []*loadbalancerv2.Member
	IsIgnored() bool
	GetID() string
}

var _ OldModelBuilder = &oldModelBuilder{}

type oldModelBuilder struct {
	oldListeners       []*oldListener
	oldPools           []*oldPool
	defaultPoolMembers []*loadbalancerv2.Member
	isIgnored          bool
	lbID               string

	annotationParser annotations.Parser
}

func (m *oldModelBuilder) GetOldListeners() []OldListener {
	oldListeners := make([]OldListener, len(m.oldListeners))
	for i, listener := range m.oldListeners {
		oldListeners[i] = listener
	}
	return oldListeners
}

func (m *oldModelBuilder) GetOldPools() []OldPool {
	oldPools := make([]OldPool, len(m.oldPools))
	for i, pool := range m.oldPools {
		oldPools[i] = pool
	}
	return oldPools
}

func (m *oldModelBuilder) GetDefaultPoolMembers() []*loadbalancerv2.Member {
	return m.defaultPoolMembers
}

func (m *oldModelBuilder) IsIgnored() bool {
	return m.isIgnored
}

func (m *oldModelBuilder) GetID() string {
	return m.lbID
}

// ------------------------------------------------------------

func NewOldModelBuilder(annos map[string]string, annotationParser annotations.Parser) OldModelBuilder {
	model := &oldModelBuilder{
		oldListeners:       make([]*oldListener, 0),
		oldPools:           make([]*oldPool, 0),
		defaultPoolMembers: make([]*loadbalancerv2.Member, 0),
		isIgnored:          false,
		lbID:               "",
		annotationParser:   annotationParser,
	}

	_ = model.build(annos)

	logrus.Debugf("Old model: %v", model)
	logrus.Debugf("Old model listeners: %v", model.GetOldListeners())
	logrus.Debugf("Old model pools: %v", model.GetOldPools())

	return model
}

func (m *oldModelBuilder) build(annos map[string]string) error {
	isIgnore := false
	if exists, err := m.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, annos); exists && err == nil {
		m.isIgnored = isIgnore
	}

	lbID := ""
	if exists := m.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &lbID, annos); exists {
		m.lbID = lbID
	}

	// build old pools
	oldPools := make([]string, 0)
	if exists := m.annotationParser.ParseStringSliceAnnotation(annotations.SuffixManagePools, &oldPools, annos); exists {
		for _, poolIDName := range oldPools {
			// poolIDName is in the format of "poolID:poolName"
			poolIDNameParts := strings.Split(poolIDName, ":")
			if len(poolIDNameParts) != 2 {
				continue
			}
			pool := &oldPool{
				commonBuilder: commonBuilder{
					id:   poolIDNameParts[0],
					name: poolIDNameParts[1],
				},
			}
			m.oldPools = append(m.oldPools, pool)
		}
	}

	// build old listeners
	oldListeners := make([]string, 0)
	if exists := m.annotationParser.ParseStringSliceAnnotation(annotations.SuffixManageListeners, &oldListeners, annos); exists {
		for _, listenerIDName := range oldListeners {
			// listenerIDName is in the format of "listenerID:listenerName:[policyName|policyName|...]"
			listenerIDNameParts := strings.Split(listenerIDName, ":")
			if len(listenerIDNameParts) != 3 {
				continue
			}

			// get policy IDs, remove the brackets
			policyNames := strings.Split(strings.Trim(listenerIDNameParts[2], "[]"), "|")
			policies := make([]OldPolicy, 0)
			for _, policyName := range policyNames {
				if policyName == "" {
					continue
				}
				policy := &oldPolicy{
					commonBuilder: commonBuilder{
						name: policyName,
					},
				}
				policies = append(policies, policy)
			}

			listener := &oldListener{
				commonBuilder: commonBuilder{
					id:   listenerIDNameParts[0],
					name: listenerIDNameParts[1],
				},
				oldPolicies: policies,
			}
			m.oldListeners = append(m.oldListeners, listener)
		}
	}

	return nil
}
