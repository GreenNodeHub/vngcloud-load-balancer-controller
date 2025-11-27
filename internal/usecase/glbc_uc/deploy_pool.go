package glbc_uc

import (
	"context"
	"fmt"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// oldPools are in Status
// newPools are in Spec
// ensure them to portal. Don't delete old pool becasue some listener is using them
func (t *defaultModelDeployTask) deployPools(ctx context.Context, lbId string) ([]v1alpha1.CreatedGlobalPool, error) {
	currentPools, err := t.vngcloudRepo.ListGlobalPools(ctx, lbId)
	if err != nil {
		return nil, err
	}

	createdPools := make([]v1alpha1.CreatedGlobalPool, 0)
	for _, pool := range t.lbConfig.Spec.GlobalPools {
		if createdPool, err := t.deployPool(ctx, lbId, &pool, currentPools); err != nil {
			return nil, err
		} else {
			createdPools = append(createdPools, *createdPool)
		}
	}
	return createdPools, nil
}

func (t *defaultModelDeployTask) deployPool(ctx context.Context, lbId string, pool *v1alpha1.GlobalPool, currentPools *entityv2.ListGlobalPools) (*v1alpha1.CreatedGlobalPool, error) {
	searchPoolByName := func(name string) *entityv2.GlobalPool {
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
		_pool, err := t.vngcloudRepo.CreateGlobalPool(ctx, lbId,
			t.buildCreatePoolRequest(ctx, lbId, pool),
		)
		if err != nil {
			return nil, err
		}
		// TODO: uncomment me
		// if err := t.statusAddPoolMember(ctx, _pool.ID, pool.Name, pool.Members); err != nil {
		// 	return nil, err
		// }

		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			return nil, err
		}
		return &v1alpha1.CreatedGlobalPool{
			Id:   _pool.ID,
			Name: pool.Name,
			// // TODO: uncomment me
			// CreatedPoolMembers: pool.Members,
		}, nil
	}

	// ensure exist pool
	updateOptions, message := t.buildPoolUpdateRequest(ctx, lbId, pool, currentPool)
	if updateOptions != nil {
		t.logger.Info("Need update pool: ", strings.Join(message, ", "))
		err := t.vngcloudRepo.UpdateGlobalPool(ctx, lbId, currentPool.ID, updateOptions)
		if err != nil {
			t.logger.Error("Failed to update pool: ", err)
			return nil, err
		}
		// TODO: uncomment me
		// if err := t.statusAddPool(ctx, currentPool.ID, currentPool.Name); err != nil {
		// 	return nil, err
		// }
		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
	}

	// ensure pool members
	currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, currentPool.ID)
	if err != nil {
		t.logger.Error("Failed to get pool members: ", err)
		return nil, err
	}

	// get created members for this pool from status
	createdMemberStatus := []v1alpha1.GlobalPoolMember{}
	for _, p := range t.lbConfig.Status.CreatedPools {
		if p.Name == pool.Name {
			createdMemberStatus = p.CreatedPoolMembers
			break
		}
	}

	updateMembers := t.mergePoolMembers(ctx,
		createdMemberStatus,
		convertMemberList(currentPoolMembers),
		pool.Members)

	if !t.comparePoolMembers(ctx, updateMembers, convertMemberList(currentPoolMembers)) {
		convertMembers := make([]loadbalancerv2.IMemberRequest, 0)
		for _, member := range updateMembers {
			convertMembers = append(convertMembers, loadbalancerv2.NewMember(member.Name, member.IP, member.Port, member.MonitorPort))
		}
		updateMemberOptions := loadbalancerv2.NewUpdatePoolMembersRequest(lbId, currentPool.UUID).WithMembers(convertMembers...)

		t.logger.Info("Need update pool members: ", updateMembers)
		if err = t.vngcloudRepo.UpdatePoolMembers(ctx, lbId, currentPool.UUID, updateMemberOptions); err != nil {
			t.logger.Error("Failed to update pool members: ", err)
			return nil, err
		}
		if err := t.statusAddPoolMember(ctx, currentPool.UUID, currentPool.Name, pool.Members); err != nil {
			return nil, err
		}
		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
	}
	return &v1alpha1.CreatedGlobalPool{
		Id:             currentPool.UUID,
		Name:           currentPool.Name,
		CreatedMembers: pool.Members,
	}, nil
}

// create CreatePoolRequest depend on default config and pool value
func (t *defaultModelDeployTask) buildCreatePoolRequest(_ context.Context, lbId string, poolSpec *v1alpha1.GlobalPool) global.ICreateGlobalPoolRequest {
	globalPoolMembersRequest := make([]global.ICreateGlobalPoolMemberRequest, len(poolSpec.Members))
	for _, poolMemberSpec := range poolSpec.Members {

		globalMembersRequest := make([]global.IGlobalMemberRequest, len(poolMemberSpec.Members))
		for _, memberSpec := range poolMemberSpec.Members {
			memberRequest := global.NewGlobalMemberRequest(
				memberSpec.Name,
				memberSpec.Address,
				memberSpec.SubnetID,
				memberSpec.Port,
				*memberSpec.MonitorPort,
				*memberSpec.Weight,
				memberSpec.BackupRole,
			)

			globalMembersRequest = append(globalMembersRequest, memberRequest)
		}

		poolMemberRequest := global.NewGlobalPoolMemberRequest(
			poolMemberSpec.Name,
			poolMemberSpec.Region,
			poolMemberSpec.VpcId,
			*poolMemberSpec.TrafficDial,
			poolMemberSpec.Type,
		)
		poolMemberRequest.WithMembers(globalMembersRequest...)
		globalPoolMembersRequest = append(globalPoolMembersRequest, poolMemberRequest)
	}

	healthMonitor := global.NewGlobalHealthMonitor(poolSpec.HealthMonitor.Protocol).
		WithHealthyThreshold(t.cfg.GlobalLoadBalancerOpts.DefaultHealthyThreshold).
		WithInterval(t.cfg.GlobalLoadBalancerOpts.DefaultInterval).
		WithTimeout(t.cfg.GlobalLoadBalancerOpts.DefaultTimeout).
		WithUnhealthyThreshold(t.cfg.GlobalLoadBalancerOpts.DefaultUnhealthyThreshold).
		// WithProtocol(pprotocol GlobalPoolHealthCheckProtocol).
		WithHealthCheckMethod(poolSpec.HealthMonitor.HealthCheckMethod).
		WithHttpVersion(poolSpec.HealthMonitor.HttpVersion).
		WithPath(poolSpec.HealthMonitor.HealthCheckPath).
		WithSuccessCode(poolSpec.HealthMonitor.SuccessCode).
		WithDomainName(poolSpec.HealthMonitor.DomainName)

	if poolSpec.HealthMonitor.HealthyThreshold != nil {
		healthMonitor.WithHealthyThreshold(*poolSpec.HealthMonitor.HealthyThreshold)
	}
	if poolSpec.HealthMonitor.UnhealthyThreshold != nil {
		healthMonitor.WithUnhealthyThreshold(*poolSpec.HealthMonitor.UnhealthyThreshold)
	}
	if poolSpec.HealthMonitor.Interval != nil {
		healthMonitor.WithInterval(*poolSpec.HealthMonitor.Interval)
	}
	if poolSpec.HealthMonitor.Timeout != nil {
		healthMonitor.WithTimeout(*poolSpec.HealthMonitor.Timeout)
	}

	r := global.NewCreateGlobalPoolRequest(poolSpec.Name, poolSpec.Protocol).
		WithAlgorithm(global.GlobalPoolAlgorithm(t.cfg.GlobalLoadBalancerOpts.DefaultPoolAlgorithm)).
		// WithDescription().
		// WithName(pname string).
		// WithProtocol(pprotocol GlobalPoolProtocol).
		WithHealthMonitor(healthMonitor).
		WithMembers(globalPoolMembersRequest...).
		WithLoadBalancerId(lbId)

	if poolSpec.Algorithm != nil && *poolSpec.Algorithm != "" {
		r.WithAlgorithm(*poolSpec.Algorithm)
	}
	return r
}

// return UpdateRequest and messages
func (t *defaultModelDeployTask) buildPoolUpdateRequest(_ context.Context, lbID string, poolSpec *v1alpha1.GlobalPool, currentPool *entityv2.GlobalPool) (global.IUpdateGlobalPoolRequest, []string) {
	isNeedUpdate := false
	message := make([]string, 0)

	healthMonitor := global.NewGlobalHealthMonitor(global.GlobalPoolHealthCheckProtocol(currentPool.Health.Protocol)).
		WithHealthyThreshold(currentPool.Health.HealthyThreshold).
		WithInterval(currentPool.Health.IntervalTime).
		WithTimeout(currentPool.Health.Timeout).
		WithUnhealthyThreshold(currentPool.Health.UnhealthyThreshold).
		// WithProtocol(pprotocol GlobalPoolHealthCheckProtocol).
		WithHealthCheckMethod((*global.GlobalPoolHealthCheckMethod)(currentPool.Health.HTTPMethod)).
		WithHttpVersion((*global.GlobalPoolHealthCheckHttpVersion)(currentPool.Health.HTTPVersion)).
		WithPath(currentPool.Health.Path).
		WithSuccessCode(currentPool.Health.SuccessCode).
		WithDomainName(currentPool.Health.DomainName)

	if poolSpec.HealthMonitor.HealthyThreshold != nil && *poolSpec.HealthMonitor.HealthyThreshold != currentPool.Health.HealthyThreshold {
		message = append(message, fmt.Sprintf("healthy threshold (%d -> %d)", currentPool.Health.HealthyThreshold, *poolSpec.HealthMonitor.HealthyThreshold))
		healthMonitor.WithHealthyThreshold(*poolSpec.HealthMonitor.HealthyThreshold)
		isNeedUpdate = true
	}
	if poolSpec.HealthMonitor.UnhealthyThreshold != nil && *poolSpec.HealthMonitor.UnhealthyThreshold != currentPool.Health.UnhealthyThreshold {
		message = append(message, fmt.Sprintf("unhealthy threshold (%d -> %d)", currentPool.Health.UnhealthyThreshold, *poolSpec.HealthMonitor.UnhealthyThreshold))
		healthMonitor.WithUnhealthyThreshold(*poolSpec.HealthMonitor.UnhealthyThreshold)
		isNeedUpdate = true
	}
	if poolSpec.HealthMonitor.Interval != nil && *poolSpec.HealthMonitor.Interval != currentPool.Health.IntervalTime {
		message = append(message, fmt.Sprintf("interval (%d -> %d)", currentPool.Health.IntervalTime, *poolSpec.HealthMonitor.Interval))
		healthMonitor.WithInterval(*poolSpec.HealthMonitor.Interval)
		isNeedUpdate = true
	}
	if poolSpec.HealthMonitor.Timeout != nil && *poolSpec.HealthMonitor.Timeout != currentPool.Health.Timeout {
		message = append(message, fmt.Sprintf("timeout (%d -> %d)", currentPool.Health.Timeout, *poolSpec.HealthMonitor.Timeout))
		healthMonitor.WithTimeout(*poolSpec.HealthMonitor.Timeout)
		isNeedUpdate = true
	}

	// compare HTTP health check options
	if string(poolSpec.HealthMonitor.Protocol) != currentPool.Health.Protocol {
		message = append(message, fmt.Sprintf("health check protocol (%s -> %s)", currentPool.Health.Protocol, poolSpec.HealthMonitor.Protocol))
		healthMonitor.WithProtocol(poolSpec.HealthMonitor.Protocol)
		isNeedUpdate = true
	}
	// switch from (HTTP, HTTPS) --> (HTTP, HTTPS)
	if (currentPool.Health.Protocol == string(global.GlobalPoolHealthCheckProtocolHTTP) || currentPool.Health.Protocol == string(global.GlobalPoolHealthCheckProtocolHTTPs)) &&
		(poolSpec.HealthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTP || poolSpec.HealthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTPs) {

		if poolSpec.HealthMonitor.HealthCheckMethod != nil && (currentPool.Health.HTTPMethod == nil || string(*poolSpec.HealthMonitor.HealthCheckMethod) != *currentPool.Health.HTTPMethod) {
			message = append(message, fmt.Sprintf("health check method (%v -> %v)", currentPool.Health.HTTPMethod, *poolSpec.HealthMonitor.HealthCheckMethod))
			healthMonitor.WithHealthCheckMethod(poolSpec.HealthMonitor.HealthCheckMethod)
			isNeedUpdate = true
		}
		if poolSpec.HealthMonitor.HealthCheckPath != nil && (currentPool.Health.Path == nil || *poolSpec.HealthMonitor.HealthCheckPath != *currentPool.Health.Path) {
			message = append(message, fmt.Sprintf("health check path (%v -> %v)", currentPool.Health.Path, *poolSpec.HealthMonitor.HealthCheckPath))
			healthMonitor.WithPath(poolSpec.HealthMonitor.HealthCheckPath)
			isNeedUpdate = true
		}
		if poolSpec.HealthMonitor.SuccessCode != nil && (currentPool.Health.SuccessCode == nil || *poolSpec.HealthMonitor.SuccessCode != *currentPool.Health.SuccessCode) {
			message = append(message, fmt.Sprintf("success code (%v -> %v)", currentPool.Health.SuccessCode, *poolSpec.HealthMonitor.SuccessCode))
			healthMonitor.WithSuccessCode(poolSpec.HealthMonitor.SuccessCode)
			isNeedUpdate = true
		}
		if poolSpec.HealthMonitor.HttpVersion != nil && (currentPool.Health.HTTPVersion == nil || string(*poolSpec.HealthMonitor.HttpVersion) != *currentPool.Health.HTTPVersion) {
			message = append(message, fmt.Sprintf("http version (%v -> %v)", currentPool.Health.HTTPVersion, *poolSpec.HealthMonitor.HttpVersion))
			healthMonitor.WithHttpVersion(poolSpec.HealthMonitor.HttpVersion)
			isNeedUpdate = true
		}
		if poolSpec.HealthMonitor.DomainName != nil && (currentPool.Health.DomainName == nil || *poolSpec.HealthMonitor.DomainName != *currentPool.Health.DomainName) {
			message = append(message, fmt.Sprintf("domain name (%v -> %v)", currentPool.Health.DomainName, *poolSpec.HealthMonitor.DomainName))
			healthMonitor.WithDomainName(poolSpec.HealthMonitor.DomainName)
			isNeedUpdate = true
		}
	} else // switch from (HTTP, HTTPS) --> (TCP)
	if (currentPool.Health.Protocol == string(global.GlobalPoolHealthCheckProtocolHTTP) || currentPool.Health.Protocol == string(global.GlobalPoolHealthCheckProtocolHTTPs)) &&
		poolSpec.HealthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolTCP {

		healthMonitor.WithHealthCheckMethod(nil)
		healthMonitor.WithPath(nil)
		healthMonitor.WithSuccessCode(nil)
		healthMonitor.WithHttpVersion(nil)
		healthMonitor.WithDomainName(nil)
		message = append(message, "health check options (http -> tcp)")
		isNeedUpdate = true

	} else // switch from (TCP) --> (HTTP, HTTPS)
	if currentPool.Health.Protocol == string(global.GlobalPoolHealthCheckProtocolTCP) &&
		(poolSpec.HealthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTP || poolSpec.HealthMonitor.Protocol == global.GlobalPoolHealthCheckProtocolHTTPs) {

		healthMonitor.WithHealthCheckMethod(poolSpec.HealthMonitor.HealthCheckMethod)
		healthMonitor.WithPath(poolSpec.HealthMonitor.HealthCheckPath)
		healthMonitor.WithSuccessCode(poolSpec.HealthMonitor.SuccessCode)
		healthMonitor.WithHttpVersion(poolSpec.HealthMonitor.HttpVersion)
		healthMonitor.WithDomainName(poolSpec.HealthMonitor.DomainName)
		message = append(message, "health check options (tcp -> http)")
		isNeedUpdate = true
	}

	updateOptions := global.NewUpdateGlobalPoolRequest(lbID, currentPool.ID).
		WithHealthMonitor(healthMonitor).WithAlgorithm(global.GlobalPoolAlgorithm(currentPool.Algorithm))

	if poolSpec.Algorithm != nil && *poolSpec.Algorithm != "" && *poolSpec.Algorithm != global.GlobalPoolAlgorithm(currentPool.Algorithm) {
		message = append(message, fmt.Sprintf("algorithm (%s -> %s)", currentPool.Algorithm, *poolSpec.Algorithm))
		updateOptions.WithAlgorithm(*poolSpec.Algorithm)
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil
	}
	return updateOptions, message
}

// MergePoolMembers merges the pool members
// - keep current member if it is in spec or not created by us
func (t *defaultModelDeployTask) mergePoolMembers(_ context.Context, createdMembers, currentMembers, poolMemberSpec []v1alpha1.PoolMember) []v1alpha1.PoolMember {
	mergedPoolMembers := make([]v1alpha1.PoolMember, 0)

	// keep current member if it is in spec or not created by us
	for _, member := range currentMembers {
		if t.checkIfPoolMemberExist(poolMemberSpec, &member) || !t.checkIfPoolMemberExist(createdMembers, &member) {
			mergedPoolMembers = append(mergedPoolMembers, member)
		}
	}

	// add new members from spec
	for _, member := range poolMemberSpec {
		if !t.checkIfPoolMemberExist(mergedPoolMembers, &member) {
			mergedPoolMembers = append(mergedPoolMembers, member)
		}
	}

	return mergedPoolMembers
}

func (t *defaultModelDeployTask) comparePoolMembers(_ context.Context, poolMembers []v1alpha1.PoolMember, current []v1alpha1.PoolMember) bool {
	if len(poolMembers) != len(current) {
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
func (t *defaultModelDeployTask) checkIfPoolMemberExist(list []v1alpha1.PoolMember, member *v1alpha1.PoolMember) bool {
	for _, r := range list {
		if r.IP == member.IP &&
			r.Port == member.Port &&
			// r.Backup == member.Backup &&
			// r.Name == member.Name &&
			// r.Weight == member.Weight &&
			r.MonitorPort == member.MonitorPort {
			return true
		}
	}
	return false
}

func convertMemberList(members *entityv2.ListMembers) []v1alpha1.PoolMember {
	result := make([]v1alpha1.PoolMember, 0)
	if members == nil || len(members.Items) == 0 {
		return result
	}
	for _, member := range members.Items {
		result = append(result, *convertMember(member))
	}
	return result
}

func convertMember(member *entityv2.Member) *v1alpha1.PoolMember {
	return &v1alpha1.PoolMember{
		Name:        member.Name,
		IP:          member.Address,
		Port:        member.ProtocolPort,
		Backup:      &member.Backup,
		Weight:      &member.Weight,
		MonitorPort: member.MonitorPort,
	}
}
