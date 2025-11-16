package builder

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

type LoadBalancerBuilder interface {
	BasicInfoHelper
	PoolListenerHelper
	NameHelper

	EnsurePool(pool *poolBuilderType, oldBuilder OldModelBuilder) error
	EnsureListener(listener *ListenerBuilderType, oldBuilder OldModelBuilder) error
	EnsureTags(tags map[string]string, oldBuilder OldModelBuilder) error
	EnsureDeleteTags(oldBuilder OldModelBuilder) error
	EnsureSecurityGroups(newBuilder ModelBuilder, oldBuilder OldModelBuilder) error
	EnsureDeleteSecurityGroups(oldBuilder OldModelBuilder) error

	//
	// DeleteRedundantListeners(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error
	// // delete redundant pools, should check if pool is used by other listeners or policy then ignore
	// DeleteRedundantPools(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error

	// CanDeleteWholeLoadBalancer(oldBuilder OldModelBuilder) bool

	// CanDeleteWholeListener(oldListener OldListener) bool

	PatchRedudantPoolMember(ModelBuilder) error

	// delete redundant pool (all empty poolmember), poolMember(no member), member(name not in whitelist)
	// delete redundant listener (point to redundant pool or no pool)
	CleanUp(context.Context, []string) error
}

// depend on loadBalancerID, get loadBalancer information by using provider, then build the ModelBuilder
func NewLoadBalancerBuilderByLoadBalancerID(
	ctx context.Context,
	loadBalancerID string,
	provider provider.Provider,
	annotationParser annotations.Parser,
	clusterID, fleetID string,
	nodes []*corev1.Node,
	object client.Object,
) (LoadBalancerBuilder, error) {
	model := &vngcloudLBBuilder{
		provider:         provider,
		context:          ctx,
		logger:           contexts.NewContext(ctx).Log(),
		annotationParser: annotationParser,
		basicInfoHelper: basicInfoHelper{
			loadBalancerID:   loadBalancerID,
			loadBalancerName: "",
			loadBalancerType: "",
			tags:             map[string]string{},
		},
		nameHelper: nameHelper{
			clusterID:         clusterID,
			fleetID:           fleetID,
			resourceType:      strings.ToLower(object.GetObjectKind().GroupVersionKind().Kind),
			resourceName:      object.GetName(),
			resourceNamespace: object.GetNamespace(),
		},
		poolListenerHelper: poolListenerHelper{
			poolBuilders:     make([]*poolBuilderType, 0),
			listenerBuilders: make([]*ListenerBuilderType, 0),
		},
		knownNodes: nodes,
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
	nameHelper

	logger           *logrus.Entry
	context          context.Context
	provider         provider.Provider
	annotationParser annotations.Parser
	knownNodes       []*corev1.Node
}

func (r *vngcloudLBBuilder) build() error {
	lb, err := r.provider.GetGlobalLoadBalancerByID(r.context, r.GetLoadBalancerID())
	if err != nil {
		return err
	}
	if lb == nil {
		return errs.NewRequeueNeeded("load balancer not have information")
	}

	r.loadBalancerID = lb.ID
	r.loadBalancerName = lb.Name
	r.loadBalancerType = global.GlobalLoadBalancerType(lb.Type)

	// Get pools
	pools, err := r.provider.ListGlobalPools(r.context, r.loadBalancerID)
	if err != nil {
		return err
	}
	for _, pool := range pools.Items {
		poolBuilder, err := r.buildPool(pool)
		if err != nil {
			return err
		}
		r.poolBuilders = append(r.poolBuilders, poolBuilder)
	}

	// Get listeners
	listeners, err := r.provider.ListGlobalListeners(r.context, r.loadBalancerID)
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

func (r *vngcloudLBBuilder) buildPool(pool *entityv2.GlobalPool) (*poolBuilderType, error) {
	poolBuilder := &poolBuilderType{
		commonBuilder: commonBuilder{
			name: pool.Name,
			id:   pool.ID,
		},
		Algorithm:     global.GlobalPoolAlgorithm(pool.Algorithm),
		Protocol:      global.GlobalPoolProtocol(pool.Protocol),
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: &global.GlobalHealthMonitorRequest{
			HealthyThreshold:    pool.Health.HealthyThreshold,
			UnhealthyThreshold:  pool.Health.UnhealthyThreshold,
			Interval:            pool.Health.IntervalTime,
			Timeout:             pool.Health.Timeout,
			Path:                pool.Health.Path,
			SuccessCode:         pool.Health.SuccessCode,
			HealthCheckProtocol: global.GlobalPoolHealthCheckProtocol(pool.Health.Protocol),
			HttpMethod:          (*global.GlobalPoolHealthCheckMethod)(pool.Health.HTTPMethod),
			HttpVersion:         (*global.GlobalPoolHealthCheckHttpVersion)(pool.Health.HTTPVersion),
			DomainName:          pool.Health.DomainName,
		},
		GlobalPoolMembers: make([]*poolMemberBuilderType, 0),
	}

	members, err := r.provider.ListGlobalPoolMembers(r.context, r.loadBalancerID, pool.ID)
	if err != nil {
		return nil, err
	}

	if members == nil || members.Items == nil {
		members = &entityv2.ListGlobalPoolMembers{
			Items: make([]*entityv2.GlobalPoolMember, 0),
		}
	}

	for _, poolMember := range members.Items {
		members := make([]global.IGlobalMemberRequest, 0)
		if poolMember.Members != nil && poolMember.Members.Items != nil {
			for _, member := range poolMember.Members.Items {
				members = append(members, &global.GlobalMemberRequest{
					Address:     member.Address,
					BackupRole:  member.BackupRole,
					Description: member.Description,
					MonitorPort: member.MonitorPort,
					Name:        member.Name,
					Port:        member.Port,
					SubnetID:    member.SubnetID,
					Weight:      member.Weight,
				})
			}
		}

		poolBuilder.GlobalPoolMembers = append(poolBuilder.GlobalPoolMembers, &poolMemberBuilderType{
			GlobalPoolMemberRequest: global.GlobalPoolMemberRequest{
				Name:        poolMember.Name,
				Description: poolMember.Description,
				Region:      poolMember.Region,
				TrafficDial: poolMember.TrafficDial,
				Type:        global.GlobalPoolMemberTypePrivate,
				VPCID:       poolMember.VpcID,
				Members:     members,
				PoolCommon: common.PoolCommon{
					PoolId: poolMember.ID,
				},
			},
			id: poolMember.ID,
		})
	}
	return poolBuilder, nil
}

func (r *vngcloudLBBuilder) buildListener(listener *entityv2.GlobalListener) (*ListenerBuilderType, error) {
	listenerBuilder := &ListenerBuilderType{
		commonBuilder: commonBuilder{
			name: listener.Name,
			id:   listener.ID,
		},
		CreateGlobalListenerRequest: global.CreateGlobalListenerRequest{
			Description:       listener.Description,
			Headers:           nil,
			Name:              listener.Name,
			Port:              listener.Port,
			Protocol:          global.GlobalListenerProtocol(listener.Protocol),
			GlobalPoolId:      listener.GlobalPoolID,
			AllowedCidrs:      listener.AllowedCidrs,
			TimeoutClient:     listener.TimeoutClient,
			TimeoutMember:     listener.TimeoutMember,
			TimeoutConnection: listener.TimeoutConnection,
		},
		isDeleted:      false,
		policyBuilders: make([]*policyBuilderType, 0),
		ReferPoolName:  listener.GlobalPoolID,
	}

	return listenerBuilder, nil
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
		if _pool, err := r.provider.CreateGlobalPool(r.context, r.GetLoadBalancerID(),
			poolBuilder.GetICreatePoolRequest(r.GetLoadBalancerID())); err != nil {
			r.logger.Error("Failed to create pool: ", err)
			return err
		} else {
			poolBuilder.SetID(_pool.ID)
		}
		if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
			r.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
		// need to update to current builder, avoid mismatch data later
		r.AddClonePoolBuilder(poolBuilder)
	} else {
		poolBuilder.SetID(poolInPortal.GetID())
		updateOptions, message := poolInPortal.ComparePoolBuilder(r.GetLoadBalancerID(), poolBuilder)
		if updateOptions != nil {
			r.logger.Info("Need update pool: ", strings.Join(message, ", "))
			err := r.provider.UpdateGlobalPool(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
				updateOptions.WithLoadBalancerId(r.GetLoadBalancerID()))
			if err != nil {
				r.logger.Error("Failed to update pool: ", err)
				return err
			}
			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}

		if patchPoolMemberOptions := r.comparePoolMembers(poolInPortal.GlobalPoolMembers, poolBuilder.GlobalPoolMembers, true); patchPoolMemberOptions != nil {
			err := r.provider.PatchGlobalPoolMember(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
				patchPoolMemberOptions)
			if err != nil {
				r.logger.Error("Failed to patch pool members: ", err)
				return err
			}
			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}
	return nil
}

func (r *vngcloudLBBuilder) EnsureListener(listenerBuilder *ListenerBuilderType, oldBuilder OldModelBuilder) error {
	// listenerInPortal := currentBuilder.GetListenerBuilderByName(listenerBuilder.GetName())
	listenerInPortal := r.GetListenerBuilderByPort(listenerBuilder.Port)
	if listenerInPortal == nil {
		if _lis, err := r.provider.CreateGlobalListener(r.context, r.GetLoadBalancerID(),
			listenerBuilder.GetICreateListenerRequest().WithLoadBalancerId(r.GetLoadBalancerID()),
		); err != nil {
			r.logger.Error("Failed to create listener: ", err)
			return err
		} else {
			listenerBuilder.SetID(_lis.ID)
		}
		if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
			r.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
		// need to update to current builder, avoid mismatch data later
		r.AddCloneListenerBuilder(listenerBuilder)
		listenerInPortal = r.GetListenerBuilderByPort(listenerBuilder.Port)
	} else {
		listenerBuilder.SetID(listenerInPortal.GetID())
		listenerBuilder.SetName(listenerInPortal.GetName())

		// if mismatch listener protocol, return error => user must delete listener in portal ..........................
		if listenerInPortal.Protocol != listenerBuilder.Protocol {
			r.logger.Error("Listener protocol mismatch: ", listenerInPortal.Protocol, listenerBuilder.Protocol)
			return nil
		}

		updateOptions, message := listenerInPortal.CompareListenerBuilder(r.GetLoadBalancerID(), listenerBuilder)
		if updateOptions != nil {
			r.logger.Info("Need update listener: ", strings.Join(message, ", "))
			err := r.provider.UpdateGlobalListener(r.context, r.GetLoadBalancerID(), listenerInPortal.GetID(), updateOptions)
			if err != nil {
				r.logger.Error("Failed to update listener: ", err)
				return err
			}

			// need to update to current builder, avoid mismatch data later
			listenerInPortal.GlobalPoolId = updateOptions.GlobalPoolId
			listenerInPortal.ReferPoolName = ""
			if p := r.GetPoolBuilderByID(updateOptions.GlobalPoolId); p != nil { // in ensurePool, should update to the latest information
				listenerInPortal.ReferPoolName = p.GetName()
			}
			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}

	return nil
}

// func (r *vngcloudLBBuilder) DeleteRedundantListeners(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error {
// 	// delete candidates include listeners in oldBuilder and all current listeners with name contains hash
// 	deleteCandidate := make(map[string]OldListener)
// 	for _, oldListener := range oldBuilder.GetOldListeners() {
// 		deleteCandidate[oldListener.GetName()] = oldListener
// 	}
// 	hash := r.GenerateHash()
// 	for _, listener := range r.GetListenerBuilders() {
// 		if strings.Contains(listener.GetName(), hash) {
// 			if ok := deleteCandidate[listener.GetName()]; ok == nil {
// 				r.logger.Infof("Add listener \"%s\" to delete candidate", listener.GetName())
// 				policies := make([]OldPolicy, 0)
// 				for _, policy := range listener.GetPolicyBuilders() {
// 					policies = append(policies, &oldPolicy{
// 						commonBuilder: commonBuilder{
// 							name: policy.GetName(),
// 							id:   policy.GetID(),
// 						},
// 					})
// 				}
// 				deleteCandidate[listener.GetName()] = &oldListener{
// 					oldPolicies: policies,
// 					commonBuilder: commonBuilder{
// 						name: listener.GetName(),
// 						id:   listener.GetID(),
// 					},
// 				}
// 			}
// 		}
// 	}

// 	// delete redundant listeners
// 	for _, oldListener := range deleteCandidate {
// 		currentListener := r.GetListenerBuilderByName(oldListener.GetName())
// 		newListener := newBuilder.GetListenerBuilderByName(oldListener.GetName())
// 		if currentListener == nil || currentListener.IsDeleted() {
// 			continue
// 		}

// 		// delete whole listener if new not used and can delete whole
// 		if newListener == nil && r.CanDeleteWholeListener(oldListener) {
// 			if err := r.provider.DeleteListener(r.context, r.GetLoadBalancerID(), oldListener.GetID()); err != nil {
// 				r.logger.Error("Failed to delete listener: ", err)
// 				return err
// 			}
// 			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
// 				r.logger.Error("Failed to wait for loadbalancer active: ", err)
// 				return err
// 			}
// 			currentListener.SetIsDeleted(true)
// 			continue
// 		}

// 		// delete redundant policy
// 		if err := r.deleteRedundantPolicies(oldListener, currentListener, newListener); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// func (r *vngcloudLBBuilder) deleteRedundantPolicies(oldListener OldListener, currentListener, newListener *ListenerBuilderType) error {
// 	// delete candidates include policies in oldListener and all current policies with name contains hash
// 	deleteCandidate := make(map[string]OldPolicy)
// 	for _, oldPolicy := range oldListener.GetOldPolicies() {
// 		deleteCandidate[oldPolicy.GetName()] = oldPolicy
// 	}
// 	hash := r.GenerateHash()
// 	for _, policy := range currentListener.GetPolicyBuilders() {
// 		if strings.Contains(policy.GetName(), hash) {
// 			if ok := deleteCandidate[policy.GetName()]; ok == nil {
// 				r.logger.Infof("Add policy \"%s\" to delete candidate", policy.GetName())
// 				deleteCandidate[policy.GetName()] = &oldPolicy{
// 					commonBuilder: commonBuilder{
// 						name: policy.GetName(),
// 						id:   policy.GetID(),
// 					},
// 				}
// 			}
// 		}
// 	}

// 	// delete redundant policy
// 	for _, policy := range deleteCandidate {
// 		currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName())
// 		var newPolicy *policyBuilderType
// 		if newListener != nil {
// 			newPolicy = newListener.GetPolicyBuilderByName(policy.GetName())
// 		}

// 		if currentPolicy == nil || currentPolicy.IsDeleted() || newPolicy != nil {
// 			continue
// 		}

// 		// delete whole policy
// 		if err := r.provider.DeletePolicy(r.context, r.GetLoadBalancerID(), currentListener.GetID(), currentPolicy.GetID()); err != nil {
// 			r.logger.Error("Failed to delete policy: ", err)
// 			return err
// 		}
// 		currentPolicy.SetIsDeleted(true)
// 		if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
// 			r.logger.Error("Failed to wait for loadbalancer active: ", err)
// 			return err
// 		}
// 	}
// 	return nil
// }

// // delete redundant pools, should check if pool is used by other listeners or policy then ignore
// func (r *vngcloudLBBuilder) DeleteRedundantPools(oldBuilder OldModelBuilder, newBuilder ModelBuilder) error {
// 	// delete candidates include pools in oldBuilder and all current pools with name contains hash
// 	deleteCandidate := make(map[string]OldPool)
// 	for _, oldPool := range oldBuilder.GetOldPools() {
// 		deleteCandidate[oldPool.GetName()] = oldPool
// 	}
// 	hash := r.GenerateHash()
// 	for _, pool := range r.GetPoolBuilders() {
// 		if strings.Contains(pool.GetName(), hash) {
// 			if ok := deleteCandidate[pool.GetName()]; ok == nil {
// 				r.logger.Infof("Add pool \"%s\" to delete candidate", pool.GetName())
// 				deleteCandidate[pool.GetName()] = &oldPool{
// 					commonBuilder: commonBuilder{
// 						name: pool.GetName(),
// 						id:   pool.GetID(),
// 					},
// 				}
// 			}
// 		}
// 	}

// 	// delete redundant pools
// 	for _, pool := range deleteCandidate {
// 		if currentPool := r.GetPoolBuilderByName(pool.GetName()); currentPool != nil &&
// 			newBuilder.GetPoolBuilderByName(pool.GetName()) == nil {
// 			if currentPool.IsDeleted() {
// 				continue
// 			}

// 			// check if this pool is default pool, should check members
// 			var (
// 				isDefaultPool  bool
// 				isMembersMatch bool
// 			)
// 			if currentPool.GetName() == domain.DEFAULT_NAME_DEFAULT_POOL {
// 				isDefaultPool = true

// 				// check if default pool members match
// 				if VNGHelper.ComparePoolMembers(oldBuilder.GetDefaultPoolMembers(), currentPool.Members, true) {
// 					isMembersMatch = true
// 				}
// 			}

// 			if isDefaultPool {
// 				if isMembersMatch {
// 					// then delete this pool
// 					if r.IsPoolInUseByOtherListener(currentPool.GetID()) {
// 						r.logger.Infof("pool \"%s\" is used by other listeners, ignore delete.", pool.GetName())
// 						continue
// 					}
// 					if err := r.provider.DeletePool(r.context, r.GetLoadBalancerID(), pool.GetID()); err != nil {
// 						r.logger.Error("Failed to delete pool: ", err)
// 						return err
// 					}
// 					currentPool.SetIsDeleted(true)
// 					if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
// 						r.logger.Error("Failed to wait for loadbalancer active: ", err)
// 						return err
// 					}
// 				} else {
// 					// just update members (delete the old members)
// 					updateOptions, err := VNGHelper.MergePoolMembers(r.GetLoadBalancerID(),
// 						oldBuilder,
// 						currentPool,
// 						nil)
// 					if err != nil {
// 						r.logger.Error("Failed to merge pool members: ", err)
// 						return err
// 					}
// 					if updateOptions == nil {
// 						return nil
// 					}
// 					err = r.provider.UpdatePoolMembers(r.context, r.GetLoadBalancerID(), currentPool.GetID(),
// 						updateOptions)
// 					if err != nil {
// 						r.logger.Error("Failed to update pool members: ", err)
// 						return err
// 					}
// 					if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
// 						r.logger.Error("Failed to wait for loadbalancer active: ", err)
// 						return err
// 					}
// 				}
// 				continue
// 			}

// 			// check as normal pool
// 			if r.IsPoolInUseByOtherListener(currentPool.GetID()) {
// 				r.logger.Infof("pool \"%s\" is used by other listeners, ignore delete.", pool.GetName())
// 				continue
// 			}
// 			if err := r.provider.DeletePool(r.context, r.GetLoadBalancerID(), pool.GetID()); err != nil {
// 				r.logger.Error("Failed to delete pool: ", err)
// 				return err
// 			}
// 			currentPool.SetIsDeleted(true)
// 			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
// 				r.logger.Error("Failed to wait for loadbalancer active: ", err)
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// use for member cluster in fleet, remove their pool member with name=clusterID but not in use
func (r *vngcloudLBBuilder) PatchRedudantPoolMember(newBuilder ModelBuilder) error {

	for _, currentPoolBuilder := range r.GetPoolBuilders() {
		wantGlobalPoolMembers := make([]*poolMemberBuilderType, 0)
		wantPoolBuilder := newBuilder.GetPoolBuilderByName(currentPoolBuilder.GetName())
		if wantPoolBuilder != nil {
			wantGlobalPoolMembers = wantPoolBuilder.GlobalPoolMembers
		}
		if patchPoolMemberOptions := r.removeRedundantMemberWithClusterID(currentPoolBuilder.GlobalPoolMembers, wantGlobalPoolMembers); patchPoolMemberOptions != nil {
			err := r.provider.PatchGlobalPoolMember(r.context, r.GetLoadBalancerID(), currentPoolBuilder.GetID(),
				patchPoolMemberOptions)
			if err != nil {
				r.logger.Error("Failed to patch pool members: ", err)
				return err
			}
			if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
				r.logger.Error("Failed to wait for loadbalancer active: ", err)
				return err
			}
		}
	}
	return nil
}

// removeRedundantMemberWithClusterID compares two pool members.
// delete all members in parentSet but not in childSet and have name=clusterID
func (l *vngcloudLBBuilder) removeRedundantMemberWithClusterID(parentSet, childSet []*poolMemberBuilderType) global.IPatchGlobalPoolMemberRequest {
	getPoolMemberByName := func(mems []*poolMemberBuilderType, name string) *poolMemberBuilderType {
		for _, mem := range mems {
			if mem.Name == name {
				return mem
			}
		}
		return nil
	}

	bulkRequests := make([]global.IBulkActionRequest, 0)

	for _, parentPM := range parentSet {
		childPM := getPoolMemberByName(childSet, parentPM.Name)
		if bulkAction := l.getIBulkActionRequest(l.clusterID, parentPM, childPM); bulkAction != nil {
			bulkRequests = append(bulkRequests, bulkAction)
		}
	}

	if len(bulkRequests) == 0 {
		return nil
	}

	return global.NewPatchGlobalPoolMemberRequest(l.GetLoadBalancerID(), ""). // set empty poolID ...
											WithBulkAction(bulkRequests...)
}

func (r *vngcloudLBBuilder) getIBulkActionRequest(clusterID string, current, want *poolMemberBuilderType) global.IBulkActionRequest {
	_want := want
	if want == nil {
		_want = &poolMemberBuilderType{
			GlobalPoolMemberRequest: global.GlobalPoolMemberRequest{
				Members: make([]global.IGlobalMemberRequest, 0),
			},
		}
	}

	if updatePoolMembersOption := r.compareGlobalPoolMembers(clusterID, current.Members, _want.Members); updatePoolMembersOption != nil {
		if len(updatePoolMembersOption) > 0 {
			return global.NewPatchGlobalPoolUpdateBulkActionRequest(current.PoolId,
				global.NewUpdateGlobalPoolMemberRequest(current.TrafficDial).WithMembers(updatePoolMembersOption...),
			)
		} else {
			return global.NewPatchGlobalPoolDeleteBulkActionRequest(current.PoolId)
		}
	}
	return nil
}

// func (r *vngcloudLBBuilder) CanDeleteWholeLoadBalancer(oldBuilder OldModelBuilder) bool {
// 	if len(oldBuilder.GetOldListeners()) < len(r.GetListenerBuilders()) {
// 		r.logger.Debugf("Can't delete whole loadbalancer, len(oldListeners) < len(currentListeners) (%d < %d)",
// 			len(oldBuilder.GetOldListeners()), len(r.GetListenerBuilders()))
// 		return false
// 	}
// 	if len(oldBuilder.GetOldPools()) < len(r.GetPoolBuilders()) {
// 		r.logger.Debugf("Can't delete whole loadbalancer, len(oldPools) < len(currentPools) (%d < %d)",
// 			len(oldBuilder.GetOldPools()), len(r.GetPoolBuilders()))
// 		return false
// 	}
// 	// if listener not exists, return false
// 	for _, listener := range oldBuilder.GetOldListeners() {
// 		currentListener := r.GetListenerBuilderByName(listener.GetName())
// 		if currentListener == nil {
// 			r.logger.Debugf("Can't delete whole loadbalancer, listener not exists: %s", listener.GetName())
// 			return false
// 		}
// 		// if policy not exists, return false
// 		currentPolicies := currentListener.GetPolicyBuilders()
// 		oldPolicies := listener.GetOldPolicies()
// 		if len(oldPolicies) < len(currentPolicies) {
// 			r.logger.Debugf("Can't delete whole loadbalancer, len(oldPolicies) < len(currentPolicies) (%d < %d)",
// 				len(oldPolicies), len(currentPolicies))
// 			return false
// 		}
// 		for _, policy := range oldPolicies {
// 			if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy == nil {
// 				r.logger.Debugf("Can't delete whole loadbalancer, policy not exists: %s", policy.GetName())
// 				return false
// 			}
// 		}
// 	}
// 	// if pool not exists, return false
// 	for _, pool := range oldBuilder.GetOldPools() {
// 		if currentPool := r.GetPoolBuilderByName(pool.GetName()); currentPool == nil {
// 			r.logger.Debugf("Can't delete whole loadbalancer, pool not exists: %s", pool.GetName())
// 			return false
// 		}
// 	}

// 	// if default pool members not match, return false
// 	if defaultPool := r.GetPoolBuilderByName(domain.DEFAULT_NAME_DEFAULT_POOL); defaultPool != nil {
// 		currentMembers := defaultPool.Members
// 		oldMembers := oldBuilder.GetDefaultPoolMembers()
// 		if len(oldMembers) < len(currentMembers) {
// 			r.logger.Debugf("Can't delete whole loadbalancer, len(oldDFMembers) < len(currentDFMembers) (%d < %d)",
// 				len(oldMembers), len(currentMembers))
// 			return false
// 		}
// 		if !VNGHelper.ComparePoolMembers(oldMembers, currentMembers, true) {
// 			r.logger.Debugf("Can't delete whole loadbalancer, default pool members not match")
// 			return false
// 		}
// 	}

// 	r.logger.Debug("Can delete whole loadbalancer")
// 	return true
// }

// func (r *vngcloudLBBuilder) CanDeleteWholeListener(oldListener OldListener) bool {
// 	currentListener := r.GetListenerBuilderByName(oldListener.GetName())
// 	if len(oldListener.GetOldPolicies()) < len(currentListener.GetPolicyBuilders()) {
// 		r.logger.Debugf("Can't delete whole listener, len(oldPolicies) < len(currentPolicies) (%d < %d)",
// 			len(oldListener.GetOldPolicies()), len(currentListener.GetPolicyBuilders()))
// 		return false
// 	}
// 	for _, currentPolicy := range currentListener.GetPolicyBuilders() {
// 		if oldPolicy := oldListener.GetOldPolicyByName(currentPolicy.GetName()); oldPolicy == nil {
// 			r.logger.Debugf("Can't delete whole listener, policy not exists: %s", currentPolicy.GetName())
// 			return false
// 		}
// 	}

// 	r.logger.Debugf("Can delete whole listener %s.", oldListener.GetName())
// 	return true
// }

func (r *vngcloudLBBuilder) DeleteListener(id string) error {
	currentListener := r.GetListenerBuilderByID(id)
	if err := r.provider.DeleteListener(r.context, r.GetLoadBalancerID(), id); err != nil {
		r.logger.Error("Failed to delete listener: ", err)
		return err
	}
	if _, err := r.provider.WaitGlobalLoadBalancerActive(r.context, r.GetLoadBalancerID()); err != nil {
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
		Algorithm:         pool.Algorithm,
		Protocol:          pool.Protocol,
		Stickiness:        pool.Stickiness,
		TLSEncryption:     pool.TLSEncryption,
		HealthMonitor:     pool.HealthMonitor,
		GlobalPoolMembers: make([]*poolMemberBuilderType, 0),
		isDeleted:         false,
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
		CreateGlobalListenerRequest: global.CreateGlobalListenerRequest{
			Description:       listener.CreateGlobalListenerRequest.Description,
			Name:              listener.CreateGlobalListenerRequest.Name,
			Port:              listener.CreateGlobalListenerRequest.Port,
			Protocol:          listener.CreateGlobalListenerRequest.Protocol,
			GlobalPoolId:      listener.CreateGlobalListenerRequest.GlobalPoolId,
			AllowedCidrs:      listener.CreateGlobalListenerRequest.AllowedCidrs,
			TimeoutClient:     listener.CreateGlobalListenerRequest.TimeoutClient,
			TimeoutConnection: listener.CreateGlobalListenerRequest.TimeoutConnection,
			TimeoutMember:     listener.CreateGlobalListenerRequest.TimeoutMember,
			Headers:           listener.CreateGlobalListenerRequest.Headers,
		},
		ReferPoolName: listener.ReferPoolName,
	}
	l.AddListenerBuilder(clone)
}

// ComparePoolMembers compares two pool members.
// mustBeEqual is true if the two pool members must be equal, otherwise, just check if the pool members exist in the other pool members.
func (l *vngcloudLBBuilder) comparePoolMembers(parentSet, childSet []*poolMemberBuilderType, mustBeEqual bool) global.IPatchGlobalPoolMemberRequest {
	getPoolMemberByName := func(mems []*poolMemberBuilderType, name string) *poolMemberBuilderType {
		for _, mem := range mems {
			if mem.Name == name {
				return mem
			}
		}
		return nil
	}

	bulkRequests := make([]global.IBulkActionRequest, 0)

	for _, childPM := range childSet {
		if parentPM := getPoolMemberByName(parentSet, childPM.Name); parentPM == nil {
			bulkRequests = append(bulkRequests,
				global.NewPatchGlobalPoolCreateBulkActionRequest(childPM))
		} else if updateRequest, _ := l.compareGlobalPoolMember(l.clusterID, &parentPM.GlobalPoolMemberRequest, &childPM.GlobalPoolMemberRequest); updateRequest != nil {
			bulkRequests = append(bulkRequests,
				global.NewPatchGlobalPoolUpdateBulkActionRequest(parentPM.PoolId, updateRequest))
		}
	}

	if len(bulkRequests) == 0 {
		return nil
	}

	return global.NewPatchGlobalPoolMemberRequest(l.GetLoadBalancerID(), ""). // set empty poolID ...
											WithBulkAction(bulkRequests...)
}

func (l *vngcloudLBBuilder) compareGlobalPoolMember(clusterID string, current, want *global.GlobalPoolMemberRequest) (global.IUpdateGlobalPoolMemberRequest, []string) {
	result := global.NewUpdateGlobalPoolMemberRequest(want.TrafficDial)
	message := make([]string, 0)
	isNeedUpdate := false

	if current.TrafficDial != want.TrafficDial {
		message = append(message, fmt.Sprintf("traffic dial (%d -> %d)", current.TrafficDial, want.TrafficDial))
		isNeedUpdate = true
	}

	if updatePoolMembersOption := l.compareGlobalPoolMembers(clusterID, current.Members, want.Members); updatePoolMembersOption != nil {
		result.WithMembers(updatePoolMembersOption...)
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil
	}

	return result, message
}

// compare all member with name=clusterID in current vs want
func (l *vngcloudLBBuilder) compareGlobalPoolMembers(clusterID string, current, want []global.IGlobalMemberRequest) []global.IGlobalMemberRequest {
	currentMembersOfCluster := make([]*global.GlobalMemberRequest, 0)
	for _, member := range current {
		_member := member.(*global.GlobalMemberRequest)
		if _member.Name == clusterID {
			currentMembersOfCluster = append(currentMembersOfCluster, _member)
		}
	}

	_want := make([]*global.GlobalMemberRequest, 0)
	for _, member := range want {
		_member := member.(*global.GlobalMemberRequest)
		_want = append(_want, _member)
	}

	if checkGlobalMemberRequestsEqual(currentMembersOfCluster, _want) {
		return nil
	}

	// remove all members with name=clusterID in current and add all members in want
	result := make([]global.IGlobalMemberRequest, 0)
	for _, member := range current {
		_member := member.(*global.GlobalMemberRequest)
		if _member.Name != clusterID {
			result = append(result, member)
		}
	}
	result = append(result, want...)
	return result
}

// check if the pool members are equal
func checkGlobalMemberRequestsEqual(mems1, mems2 []*global.GlobalMemberRequest) bool {
	if len(mems1) != len(mems2) {
		return false
	}
	for _, mem := range mems1 {
		if !checkGlobalMemberRequestExist(mems2, mem) {
			return false
		}
	}
	return true
}

func checkGlobalMemberRequestExist(mems []*global.GlobalMemberRequest, mem *global.GlobalMemberRequest) bool {
	for _, r := range mems {
		if r.Address == mem.Address &&
			r.Port == mem.Port &&
			r.Weight == mem.Weight &&
			r.BackupRole == mem.BackupRole &&
			r.SubnetID == mem.SubnetID &&
			r.MonitorPort == mem.MonitorPort {
			return true
		}
	}
	return false
}
