package vngcloud_repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	computev2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/compute/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (r *vngCloudRepository) GetSubnetByID(ctx context.Context, networkID, subnetID string) (*entityv2.Subnet, error) {
	logger := contexts.NewContext(ctx).Log()
	subnet, sdkErr := r.client.VServerGateway().V2().NetworkService().GetSubnetById(networkv2.NewGetSubnetByIdRequest(networkID, subnetID).AddUserAgent(r.userAgent))
	if sdkErr != nil {
		logger.Debug("GetSubnetByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return subnet, nil
}

func (m *vngCloudRepository) GetServerNetworkInfo(ctx context.Context, instanceID string) (zoneID common.Zone, networkID, subnetID, subnetCIDR string, err error) {
	logger := contexts.NewContext(ctx).Log()

	if instanceID == "" {
		logger.Debug("GetServerNetworkInfo: serverID is empty")
		return "", "", "", "", domain.ErrorInvalidInput
	}

	if cached, ok := m.serverNetworkCache.get(instanceID); ok {
		return cached.zoneID, cached.networkID, cached.subnetID, cached.subnetCIDR, nil
	}

	server, sdkErr := m.GetServerByID(ctx, instanceID)
	if sdkErr != nil {
		return "", "", "", "", sdkErr
	}
	if server == nil {
		return "", "", "", "", domain.ErrorNotFound
	}

	networkID = server.InternalInterfaces[0].NetworkUuid
	subnetID = server.InternalInterfaces[0].SubnetUuid
	zoneID = common.Zone(server.ZoneId)

	if subnetID == "" {
		logger.Debugf("GetServerNetworkInfo: server %s has no subnet ID on its first interface", instanceID)
		return "", "", "", "", domain.ErrorNotFound
	}

	subnetCIDR, err = m.getSubnetCIDR(ctx, networkID, subnetID)
	if err != nil {
		return "", "", "", "", err
	}

	// Only successful lookups are remembered: caching a NotFound would keep a node that
	// is still being created unreachable for the whole TTL.
	m.serverNetworkCache.put(instanceID, serverNetworkInfo{
		zoneID:     zoneID,
		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: subnetCIDR,
	})

	return zoneID, networkID, subnetID, subnetCIDR, nil
}

func (m *vngCloudRepository) GetServerByID(ctx context.Context, serverID string) (*entityv2.Server, error) {
	logger := contexts.NewContext(ctx).Log()

	opt := computev2.NewGetServerByIdRequest(serverID)
	server, sdkErr := m.client.VServerGateway().V2().ComputeService().GetServerById(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("GetServerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return server, nil
}

func (m *vngCloudRepository) getSubnetCIDR(ctx context.Context, networkId, subnetID string) (string, error) {
	subnet, err := m.GetSubnetByID(ctx, networkId, subnetID)
	if err != nil {
		return "", err
	}
	if subnet == nil {
		return "", domain.ErrorNotFound
	}
	return subnet.Cidr, nil
}

func (m *vngCloudRepository) ListServerBySecgroupID(ctx context.Context, secgroupID string) (*entityv2.ListServers, error) {
	logger := contexts.NewContext(ctx).Log()

	opt := networkv2.NewListAllServersBySecgroupIdRequest(secgroupID)
	servers, sdkErr := m.client.VServerGateway().V2().NetworkService().ListAllServersBySecgroupId(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListServerBySecgroupID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return servers, nil
}

func (m *vngCloudRepository) WaitForServerActive(ctx context.Context, serverID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Waiting for server %s to be ready", serverID)

	var lastStatus string
	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		server, _err := m.GetServerByID(ctx, serverID)
		if _err != nil {
			logger.Debugf("Error getting server %s when wait active: %v", serverID, _err)
			return false, _err
		}
		lastStatus = server.Status
		if strings.ToUpper(server.Status) == consts.ACTIVE_LOADBALANCER_STATUS {
			logger.Debugf("Server %s is ready", serverID)
			return true, nil
		}
		if strings.ToUpper(server.Status) == consts.ERROR_LOADBALANCER_STATUS {
			return true, fmt.Errorf("server %s status is ERROR", serverID)
		}

		logger.Debugf("Server %s is not ready yet, waiting...", serverID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for server %s to become active, last status %q", serverID, lastStatus)
	}

	return err
}
