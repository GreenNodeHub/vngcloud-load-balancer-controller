package glbc_uc

import (
	"context"
	"fmt"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"

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

func (t *defaultModelDeployTask) deployPool(ctx context.Context, lbId string, poolSpec *v1alpha1.GlobalPool, currentPools *entityv2.ListGlobalPools) (*v1alpha1.CreatedGlobalPool, error) {
	searchPoolByName := func(name string) *entityv2.GlobalPool {
		for _, p := range currentPools.Items {
			if p.Name == name {
				return p
			}
		}
		return nil
	}

	currentPool := searchPoolByName(poolSpec.Name)
	if currentPool == nil {
		// Create new pool
		_pool, err := t.vngcloudRepo.CreateGlobalPool(ctx, lbId,
			t.buildCreatePoolRequest(ctx, lbId, poolSpec),
		)
		if err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
			return nil, err
		}

		// Fetch API-assigned member data (CreateGlobalPool response does not include member IDs)
		currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, _pool.ID)
		if err != nil {
			return nil, err
		}

		searchPoolMemberByName := func(name string) *entityv2.GlobalPoolMember {
			for _, p := range currentPoolMembers.Items {
				if p.Name == name {
					return p
				}
			}
			return nil
		}

		createdPoolMembers := make([]v1alpha1.CreatedGlobalPoolMember, 0)
		for _, poolMemberSpec := range poolSpec.PoolMembers {
			if currentPoolMember := searchPoolMemberByName(poolMemberSpec.Name); currentPoolMember != nil {
				createdMembers := make([]v1alpha1.GlobalMember, 0)
				for _, member := range currentPoolMember.Members.Items {
					createdMembers = append(createdMembers, v1alpha1.GlobalMember{
						Name:        member.Name,
						Address:     member.Address,
						Port:        member.Port,
						BackupRole:  member.BackupRole,
						Weight:      &member.Weight,
						MonitorPort: &member.MonitorPort,
						SubnetID:    member.SubnetID,
					})
				}
				createdPoolMembers = append(createdPoolMembers, v1alpha1.CreatedGlobalPoolMember{
					Id:             currentPoolMember.ID,
					Name:           currentPoolMember.Name,
					CreatedMembers: createdMembers,
				})
			}
		}

		if err := t.statusUpdatePoolMember(ctx, _pool.ID, poolSpec.Name, createdPoolMembers); err != nil {
			return nil, err
		}

		return &v1alpha1.CreatedGlobalPool{
			Id:                 _pool.ID,
			Name:               poolSpec.Name,
			CreatedPoolMembers: createdPoolMembers,
		}, nil
	}

	// ensure exist pool (spec + health monitor, not members)
	updateOptions, message := t.buildPoolUpdateRequest(ctx, lbId, poolSpec, currentPool)
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
	createdPoolMembers, err := t.deployPoolMembers(ctx, lbId, currentPool.ID, currentPool.Name, poolSpec.PoolMembers)
	if err != nil {
		return nil, err
	}

	return &v1alpha1.CreatedGlobalPool{
		Id:                 currentPool.ID,
		Name:               currentPool.Name,
		CreatedPoolMembers: createdPoolMembers,
	}, nil
}

// create CreatePoolRequest depend on default config and pool value
func (t *defaultModelDeployTask) buildCreatePoolRequest(_ context.Context, lbId string, poolSpec *v1alpha1.GlobalPool) global.ICreateGlobalPoolRequest {
	globalPoolMembersRequest := make([]global.ICreateGlobalPoolMemberRequest, 0, len(poolSpec.PoolMembers))
	for _, poolMemberSpec := range poolSpec.PoolMembers {

		globalMembersRequest := make([]global.IGlobalMemberRequest, 0, len(poolMemberSpec.Members))
		for _, memberSpec := range poolMemberSpec.Members {
			// Use default values for optional fields
			monitorPort := memberSpec.Port // default to member port
			if memberSpec.MonitorPort != nil {
				monitorPort = *memberSpec.MonitorPort
			}
			weight := 1 // default weight
			if memberSpec.Weight != nil {
				weight = *memberSpec.Weight
			}

			memberRequest := global.NewGlobalMemberRequest(
				memberSpec.Name,
				memberSpec.Address,
				memberSpec.SubnetID,
				memberSpec.Port,
				monitorPort,
				weight,
				memberSpec.BackupRole,
			)

			globalMembersRequest = append(globalMembersRequest, memberRequest)
		}

		// Use default traffic dial if not specified
		trafficDial := 100 // default to 100%
		if poolMemberSpec.TrafficDial != nil {
			trafficDial = *poolMemberSpec.TrafficDial
		}

		poolMemberRequest := global.NewGlobalPoolMemberRequest(
			poolMemberSpec.Name,
			poolMemberSpec.Region,
			poolMemberSpec.VpcId,
			trafficDial,
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

// buildPoolUpdateRequest only compare pool's spec and heath monitor
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
