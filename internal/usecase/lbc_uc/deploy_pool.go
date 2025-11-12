package lbc_uc

import (
	"context"
	"fmt"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

// oldPools are in Status
// newPools are in Spec
// ensure them to portal. Don't delete old pool becasue some listener is using them
func (t *defaultModelDeployTask) deployPools(ctx context.Context, lbId string) ([]v1alpha1.CreatedPool, error) {
	currentPools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		return nil, err
	}

	createdPools := make([]v1alpha1.CreatedPool, 0)
	for _, pool := range t.lbConfig.Spec.Pools {
		if createdPool, err := t.deployPool(ctx, lbId, &pool, currentPools); err != nil {
			return nil, err
		} else {
			createdPools = append(createdPools, *createdPool)
		}
	}
	return createdPools, nil
}

func (t *defaultModelDeployTask) deployPool(ctx context.Context, lbId string, pool *v1alpha1.Pool, currentPools *entityv2.ListPools) (*v1alpha1.CreatedPool, error) {
	searchPoolByName := func(name string) *entityv2.Pool {
		for _, p := range currentPools.Items {
			if p.Name == name {
				return p
			}
		}
		return nil
	}

	currentPool := searchPoolByName(pool.Name)
	if currentPool == nil {
		// Create new pool
		_pool, err := t.vngcloudRepo.CreatePool(ctx, lbId,
			t.buildCreatePoolRequest(ctx, lbId, pool),
		)
		if err != nil {
			return nil, err
		}
		if err := t.statusAddPool(ctx, _pool.UUID, pool.Name); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			return nil, err
		}
		return &v1alpha1.CreatedPool{
			Id:   _pool.UUID,
			Name: pool.Name,
		}, nil
	}

	// get health monitor info
	healthMonitor, err := t.vngcloudRepo.GetPoolHealthMonitorById(ctx, lbId, currentPool.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get health monitor for pool %s: %v", currentPool.UUID, err)
	}
	currentPool.HealthMonitor = healthMonitor

	// update exist pool
	updateOptions, message := t.buildPoolUpdateRequest(ctx, lbId, pool, currentPool)
	if updateOptions != nil {
		t.logger.Info("Need update pool: ", strings.Join(message, ", "))
		err := t.vngcloudRepo.UpdatePool(ctx, lbId, currentPool.UUID, updateOptions)
		if err != nil {
			t.logger.Error("Failed to update pool: ", err)
			return nil, err
		}
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
	}

	// update pool members
	currentPoolMembers, err := t.vngcloudRepo.GetPoolMembers(ctx, lbId, currentPool.UUID)
	if err != nil {
		t.logger.Error("Failed to get pool members: ", err)
		return nil, err
	}

	// ensure pool members, with default pool, should merge pool members, otherwise, should update
	if currentPool.Name == consts.DEFAULT_NAME_DEFAULT_POOL {
		// TODO
		// updateOptions, err := t.mergePoolMembers(t.GetLoadBalancerID(),
		// 	oldBuilder,
		// 	poolInPortal,
		// 	poolBuilder)
		// if err != nil {
		// 	r.logger.Error("Failed to merge pool members: ", err)
		// 	return err
		// }
		// if updateOptions == nil {
		// 	return nil
		// }
		// err = r.provider.UpdatePoolMembers(r.context, r.GetLoadBalancerID(), poolInPortal.GetID(),
		// 	updateOptions)
		// if err != nil {
		// 	r.logger.Error("Failed to update pool members: ", err)
		// 	return err
		// }
		// if _, err := r.provider.WaitForLBActive(r.context, r.GetLoadBalancerID()); err != nil {
		// 	r.logger.Error("Failed to wait for loadbalancer active: ", err)
		// 	return err
		// }
	} else // normal pool
	if !t.comparePoolMembers(ctx, pool.Members, currentPoolMembers) {
		currentPoolString := make([]string, 0)
		for _, m := range currentPoolMembers.Items {
			currentPoolString = append(currentPoolString, fmt.Sprintf("%s:%d", m.Address, m.ProtocolPort))
		}
		desiredPoolString := make([]string, 0)
		for _, m := range pool.Members {
			desiredPoolString = append(desiredPoolString, fmt.Sprintf("%s:%d", m.IP, m.Port))
		}
		t.logger.Debugf("Current pool members: %+v", currentPoolString)
		t.logger.Debugf("Desired pool members: %+v", desiredPoolString)

		err := t.vngcloudRepo.UpdatePoolMembers(ctx, lbId, currentPool.UUID,
			t.buildPoolMemberUpdateRequest(ctx, lbId, currentPool.UUID, pool))
		if err != nil {
			t.logger.Error("Failed to update pool members: ", err)
			return nil, err
		}
		if err := t.statusAddPool(ctx, currentPool.UUID, currentPool.Name); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
	}
	return &v1alpha1.CreatedPool{
		Id:   currentPool.UUID,
		Name: currentPool.Name,
	}, nil
}

// create CreatePoolRequest depend on default config and pool value
func (t *defaultModelDeployTask) buildCreatePoolRequest(_ context.Context, lbId string, pool *v1alpha1.Pool) loadbalancerv2.ICreatePoolRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range pool.Members {
		convertMembers = append(convertMembers,
			loadbalancerv2.NewMember(
				member.Name,
				member.IP,
				member.Port,
				member.MonitorPort,
			))
	}
	healthMonitor := loadbalancerv2.HealthMonitor{
		HealthCheckProtocol: pool.HealthMonitor.Protocol,
		HealthyThreshold:    t.cfg.LoadBalancerOpts.DefaultHealthyThreshold,
		UnhealthyThreshold:  t.cfg.LoadBalancerOpts.DefaultUnhealthyThreshold,
		Interval:            t.cfg.LoadBalancerOpts.DefaultInterval,
		Timeout:             t.cfg.LoadBalancerOpts.DefaultTimeout,

		HealthCheckMethod: pool.HealthMonitor.HealthCheckMethod,
		HealthCheckPath:   pool.HealthMonitor.HealthCheckPath,
		SuccessCode:       pool.HealthMonitor.SuccessCode,
		HttpVersion:       pool.HealthMonitor.HttpVersion,
		DomainName:        pool.HealthMonitor.DomainName,
	}
	if pool.HealthMonitor.HealthyThreshold != nil {
		healthMonitor.HealthyThreshold = *pool.HealthMonitor.HealthyThreshold
	}
	if pool.HealthMonitor.UnhealthyThreshold != nil {
		healthMonitor.UnhealthyThreshold = *pool.HealthMonitor.UnhealthyThreshold
	}
	if pool.HealthMonitor.Interval != nil {
		healthMonitor.Interval = *pool.HealthMonitor.Interval
	}
	if pool.HealthMonitor.Timeout != nil {
		healthMonitor.Timeout = *pool.HealthMonitor.Timeout
	}

	r := &loadbalancerv2.CreatePoolRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{LoadBalancerId: lbId},
		Algorithm:          loadbalancerv2.PoolAlgorithm(t.cfg.LoadBalancerOpts.DefaultPoolAlgorithm),
		PoolName:           pool.Name,
		PoolProtocol:       pool.Protocol,
		Stickiness:         nil,
		TLSEncryption:      nil,
		HealthMonitor:      &healthMonitor,
		Members:            convertMembers,
	}
	if pool.Algorithm != nil && *pool.Algorithm != "" {
		r.Algorithm = *pool.Algorithm
	}
	return r
}

// return UpdateRequest and messages
func (t *defaultModelDeployTask) buildPoolUpdateRequest(_ context.Context, lbID string, pool *v1alpha1.Pool, current *entityv2.Pool) (*loadbalancerv2.UpdatePoolRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)

	healthMonitor := &loadbalancerv2.HealthMonitor{
		HealthyThreshold:    current.HealthMonitor.HealthyThreshold,
		UnhealthyThreshold:  current.HealthMonitor.UnhealthyThreshold,
		Interval:            current.HealthMonitor.Interval,
		Timeout:             current.HealthMonitor.Timeout,
		HealthCheckProtocol: loadbalancerv2.HealthCheckProtocol(current.HealthMonitor.HealthCheckProtocol),
		HealthCheckPath:     current.HealthMonitor.HealthCheckPath,
		DomainName:          current.HealthMonitor.DomainName,
		SuccessCode:         current.HealthMonitor.SuccessCode,
		HealthCheckMethod: func() *loadbalancerv2.HealthCheckMethod {
			if current.HealthMonitor != nil && current.HealthMonitor.HealthCheckMethod != nil {
				return ptr.To(loadbalancerv2.HealthCheckMethod(*current.HealthMonitor.HealthCheckMethod))
			}
			return nil
		}(),
		HttpVersion: func() *loadbalancerv2.HealthCheckHttpVersion {
			if current.HealthMonitor != nil && current.HealthMonitor.HttpVersion != nil {
				return ptr.To(loadbalancerv2.HealthCheckHttpVersion(*current.HealthMonitor.HttpVersion))
			}
			return nil
		}(),
	}
	updateOptions := &loadbalancerv2.UpdatePoolRequest{
		PoolCommon: common.PoolCommon{
			PoolId: current.UUID,
		},
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbID,
		},
		Algorithm:     loadbalancerv2.PoolAlgorithm(current.LoadBalanceMethod),
		Stickiness:    nil,
		TLSEncryption: nil,
		HealthMonitor: healthMonitor,
	}
	if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer7 {
		updateOptions.Stickiness = &current.Stickiness
		updateOptions.TLSEncryption = &current.TLSEncryption

		if pool.Stickiness != nil && *pool.Stickiness != current.Stickiness {
			message = append(message, fmt.Sprintf("stickiness (%t -> %t)", current.Stickiness, *pool.Stickiness))
			updateOptions.Stickiness = ptr.To(*pool.Stickiness)
			isNeedUpdate = true
		}
		if pool.TLSEncryption != nil && *pool.TLSEncryption != current.TLSEncryption {
			message = append(message, fmt.Sprintf("tls encryption (%t -> %t)", current.TLSEncryption, *pool.TLSEncryption))
			updateOptions.TLSEncryption = ptr.To(*pool.TLSEncryption)
			isNeedUpdate = true
		}
	}
	if pool.Algorithm != nil && *pool.Algorithm != "" && *pool.Algorithm != loadbalancerv2.PoolAlgorithm(current.LoadBalanceMethod) {
		message = append(message, fmt.Sprintf("algorithm (%s -> %s)", current.LoadBalanceMethod, *pool.Algorithm))
		updateOptions.WithAlgorithm(*pool.Algorithm)
		isNeedUpdate = true
	}

	if pool.HealthMonitor.HealthyThreshold != nil && *pool.HealthMonitor.HealthyThreshold != current.HealthMonitor.HealthyThreshold {
		message = append(message, fmt.Sprintf("healthy threshold (%d -> %d)", current.HealthMonitor.HealthyThreshold, *pool.HealthMonitor.HealthyThreshold))
		updateOptions.HealthMonitor.WithHealthyThreshold(*pool.HealthMonitor.HealthyThreshold)
		isNeedUpdate = true
	}
	if pool.HealthMonitor.UnhealthyThreshold != nil && *pool.HealthMonitor.UnhealthyThreshold != current.HealthMonitor.UnhealthyThreshold {
		message = append(message, fmt.Sprintf("unhealthy threshold (%d -> %d)", current.HealthMonitor.UnhealthyThreshold, *pool.HealthMonitor.UnhealthyThreshold))
		updateOptions.HealthMonitor.WithUnhealthyThreshold(*pool.HealthMonitor.UnhealthyThreshold)
		isNeedUpdate = true
	}
	if pool.HealthMonitor.Interval != nil && *pool.HealthMonitor.Interval != current.HealthMonitor.Interval {
		message = append(message, fmt.Sprintf("interval (%d -> %d)", current.HealthMonitor.Interval, *pool.HealthMonitor.Interval))
		updateOptions.HealthMonitor.WithInterval(*pool.HealthMonitor.Interval)
		isNeedUpdate = true
	}
	if pool.HealthMonitor.Timeout != nil && *pool.HealthMonitor.Timeout != current.HealthMonitor.Timeout {
		message = append(message, fmt.Sprintf("timeout (%d -> %d)", current.HealthMonitor.Timeout, *pool.HealthMonitor.Timeout))
		updateOptions.HealthMonitor.WithTimeout(*pool.HealthMonitor.Timeout)
		isNeedUpdate = true
	}

	// compare HTTP health check options
	if string(pool.HealthMonitor.Protocol) != current.HealthMonitor.HealthCheckProtocol {
		message = append(message, fmt.Sprintf("health check protocol (%s -> %s)", current.HealthMonitor.HealthCheckProtocol, pool.HealthMonitor.Protocol))
		updateOptions.HealthMonitor.WithHealthCheckProtocol(pool.HealthMonitor.Protocol)
		isNeedUpdate = true
	}
	// switch from (HTTP, HTTPS) --> (HTTP, HTTPS)
	if (current.HealthMonitor.HealthCheckProtocol == string(loadbalancerv2.HealthCheckProtocolHTTP) || current.HealthMonitor.HealthCheckProtocol == string(loadbalancerv2.HealthCheckProtocolHTTPs)) &&
		(pool.HealthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolHTTP || pool.HealthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolHTTPs) {

		if pool.HealthMonitor.HealthCheckMethod != nil && (current.HealthMonitor.HealthCheckMethod == nil || string(*pool.HealthMonitor.HealthCheckMethod) != *current.HealthMonitor.HealthCheckMethod) {
			message = append(message, fmt.Sprintf("health check method (%v -> %v)", current.HealthMonitor.HealthCheckMethod, *pool.HealthMonitor.HealthCheckMethod))
			updateOptions.HealthMonitor.WithHealthCheckMethod(pool.HealthMonitor.HealthCheckMethod)
			isNeedUpdate = true
		}
		if pool.HealthMonitor.HealthCheckPath != nil && (current.HealthMonitor.HealthCheckPath == nil || *pool.HealthMonitor.HealthCheckPath != *current.HealthMonitor.HealthCheckPath) {
			message = append(message, fmt.Sprintf("health check path (%v -> %v)", current.HealthMonitor.HealthCheckPath, *pool.HealthMonitor.HealthCheckPath))
			updateOptions.HealthMonitor.WithHealthCheckPath(pool.HealthMonitor.HealthCheckPath)
			isNeedUpdate = true
		}
		if pool.HealthMonitor.SuccessCode != nil && (current.HealthMonitor.SuccessCode == nil || *pool.HealthMonitor.SuccessCode != *current.HealthMonitor.SuccessCode) {
			message = append(message, fmt.Sprintf("success code (%v -> %v)", current.HealthMonitor.SuccessCode, *pool.HealthMonitor.SuccessCode))
			updateOptions.HealthMonitor.WithSuccessCode(pool.HealthMonitor.SuccessCode)
			isNeedUpdate = true
		}
		if pool.HealthMonitor.HttpVersion != nil && (current.HealthMonitor.HttpVersion == nil || string(*pool.HealthMonitor.HttpVersion) != *current.HealthMonitor.HttpVersion) {
			message = append(message, fmt.Sprintf("http version (%v -> %v)", current.HealthMonitor.HttpVersion, *pool.HealthMonitor.HttpVersion))
			updateOptions.HealthMonitor.WithHttpVersion(pool.HealthMonitor.HttpVersion)
			isNeedUpdate = true
		}
		if pool.HealthMonitor.DomainName != nil && (current.HealthMonitor.DomainName == nil || *pool.HealthMonitor.DomainName != *current.HealthMonitor.DomainName) {
			message = append(message, fmt.Sprintf("domain name (%v -> %v)", current.HealthMonitor.DomainName, *pool.HealthMonitor.DomainName))
			updateOptions.HealthMonitor.WithDomainName(pool.HealthMonitor.DomainName)
			isNeedUpdate = true
		}
	} else // switch from (HTTP, HTTPS) --> (TCP)
	if (current.HealthMonitor.HealthCheckProtocol == string(loadbalancerv2.HealthCheckProtocolHTTP) || current.HealthMonitor.HealthCheckProtocol == string(loadbalancerv2.HealthCheckProtocolHTTPs)) &&
		pool.HealthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolTCP {

		updateOptions.HealthMonitor.WithHealthCheckMethod(nil)
		updateOptions.HealthMonitor.WithHealthCheckPath(nil)
		updateOptions.HealthMonitor.WithSuccessCode(nil)
		updateOptions.HealthMonitor.WithHttpVersion(nil)
		updateOptions.HealthMonitor.WithDomainName(nil)
		message = append(message, "health check options (http -> tcp)")
		isNeedUpdate = true

	} else // switch from (TCP) --> (HTTP, HTTPS)
	if current.HealthMonitor.HealthCheckProtocol == string(loadbalancerv2.HealthCheckProtocolTCP) &&
		(pool.HealthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolHTTP || pool.HealthMonitor.Protocol == loadbalancerv2.HealthCheckProtocolHTTPs) {

		updateOptions.HealthMonitor.WithHealthCheckMethod(pool.HealthMonitor.HealthCheckMethod)
		updateOptions.HealthMonitor.WithHealthCheckPath(pool.HealthMonitor.HealthCheckPath)
		updateOptions.HealthMonitor.WithSuccessCode(pool.HealthMonitor.SuccessCode)
		updateOptions.HealthMonitor.WithHttpVersion(pool.HealthMonitor.HttpVersion)
		updateOptions.HealthMonitor.WithDomainName(pool.HealthMonitor.DomainName)
		message = append(message, "health check options (tcp -> http)")
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

func (t *defaultModelDeployTask) comparePoolMembers(_ context.Context, poolMembers []v1alpha1.PoolMember, current *entityv2.ListMembers) bool {
	if len(poolMembers) != len(current.Items) {
		return false
	}

	for _, member := range poolMembers {
		if !t.checkIfPoolMemberExist(current, &member) {
			return false
		}
	}

	return true
}

// checkIfPoolMemberExist checks if the pool member exists in the pool members.
func (t *defaultModelDeployTask) checkIfPoolMemberExist(current *entityv2.ListMembers, member *v1alpha1.PoolMember) bool {
	for _, r := range current.Items {
		if r.Address == member.IP &&
			r.ProtocolPort == member.Port &&
			// r.Backup == member.Backup &&
			// r.Name == member.Name &&
			// r.Weight == member.Weight &&
			r.MonitorPort == member.MonitorPort {
			return true
		}
	}
	return false
}

func (t *defaultModelDeployTask) buildPoolMemberUpdateRequest(_ context.Context, lbID, poolId string, pool *v1alpha1.Pool) loadbalancerv2.IUpdatePoolMembersRequest {
	convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
	for _, member := range pool.Members {
		convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IP, member.Port, member.MonitorPort))
	}
	return loadbalancerv2.NewUpdatePoolMembersRequest(lbID, poolId).WithMembers(convertMembers...)
}

// delete pools created not in use anymore
// should check if pool is used by other listeners or policies (user use) then ignore
func (t *defaultModelDeployTask) deployDeleteRedundantPools(ctx context.Context, lbId string, status v1alpha1.LoadBalancerConfigStatus) error {
	deleteCandidates := make([]string, 0)
	for _, pool := range status.CreatedPools {
		deleteCandidates = append(deleteCandidates, pool.Id)
	}

	currentPools, err := t.vngcloudRepo.ListPool(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list pools of load balancer: ", err)
		return err
	}
	isPoolExist := func(poolId string) bool {
		for _, p := range currentPools.Items {
			if p.UUID == poolId {
				return true
			}
		}
		return false
	}

	// find pools in use by listeners and policies
	mapPoolInUse := make(map[string]bool)
	currentListeners, err := t.vngcloudRepo.ListListenerOfLB(ctx, lbId)
	if err != nil {
		t.logger.Error("Failed to list listeners of load balancer: ", err)
		return err
	}
	for _, listener := range currentListeners.Items {
		mapPoolInUse[listener.DefaultPoolId] = true
		if t.lbConfig.Spec.Type == loadbalancerv2.LoadBalancerTypeLayer7 {
			// check listener policies
			policies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listener.UUID)
			if err != nil {
				t.logger.Error("Failed to list policies of listener: ", err)
				return err
			}
			for _, policy := range policies.Items {
				if policy.RedirectPoolID != "" {
					mapPoolInUse[policy.RedirectPoolID] = true
				}
			}
		}
	}

	isPoolInUse := func(poolId string) bool {
		if _, ok := mapPoolInUse[poolId]; ok {
			return true
		}
		return false
	}

	for _, candidateId := range deleteCandidates {
		if isPoolInUse(candidateId) {
			continue
		}
		if !isPoolExist(candidateId) {
			t.logger.Warnf("Pool %s not found in load balancer %s, skip delete", candidateId, lbId)
			continue
		}

		// delete pool
		err := t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)
		if err != nil {
			t.logger.Error("Failed to delete pool: ", err)
			return err
		}
		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}
	return nil
}
