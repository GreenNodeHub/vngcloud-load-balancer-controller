package vngcloud_repo

import (
	"context"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"k8s.io/apimachinery/pkg/util/wait"
)

// --------------------------- Global Load Balancer ---------------------------

func (m *vngCloudRepository) ListGlobalPackages(ctx context.Context) (*entityv2.ListGlobalPackages, error) {
	logger := contexts.NewContext(ctx).Log()
	packages, sdkErr := m.client.GLBGateway().V1().GLBService().ListGlobalPackages(global.NewListGlobalPackagesRequest().AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListGlobalPackages: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return packages, nil
}

func (m *vngCloudRepository) ListGlobalLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListGlobalLoadBalancers, error) {
	logger := contexts.NewContext(ctx).Log()
	lbs, sdkErr := m.client.GLBGateway().V1().GLBService().ListGlobalLoadBalancers(global.NewListGlobalLoadBalancersRequest(defaultOffset, defaultPageSize).WithTags(tags...).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListGlobalLoadBalancers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return lbs, nil
}

func (m *vngCloudRepository) GetGlobalLoadBalancerByID(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	lb, sdkErr := m.client.GLBGateway().V1().GLBService().GetGlobalLoadBalancerById(global.NewGetGlobalLoadBalancerByIdRequest(glbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("GetGlobalLoadBalancerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return lb, nil
}

func (m *vngCloudRepository) GetGlobalLoadBalancerByName(ctx context.Context, name string) (*entityv2.GlobalLoadBalancer, error) {
	allGLBs, err := m.ListGlobalLoadBalancers(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, glb := range allGLBs.Items {
		if glb.Name == name {
			return glb, nil
		}
	}
	return nil, nil
}

func (m *vngCloudRepository) CreateGlobalLoadBalancer(ctx context.Context, glbOptions global.ICreateGlobalLoadBalancerRequest) (*entityv2.GlobalLoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create global load balancer %v", glbOptions.ToMap()["name"])
	lb, sdkErr := m.client.GLBGateway().V1().GLBService().CreateGlobalLoadBalancer(glbOptions.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateGlobalLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return lb, nil
}

func (m *vngCloudRepository) DeleteGlobalLoadBalancer(ctx context.Context, glbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete global load balancer %s", glbID)
	err := m.client.GLBGateway().V1().GLBService().DeleteGlobalLoadBalancer(global.NewDeleteGlobalLoadBalancerRequest(glbID).AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("DeleteGlobalLoadBalancer: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}

func (m *vngCloudRepository) WaitGlobalLoadBalancerActive(ctx context.Context, glbID string) (*entityv2.GlobalLoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Waiting for global load balancer %s to be ready", glbID)
	var resultGLB *entityv2.GlobalLoadBalancer
	var lastStatus string

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetGlobalLoadBalancerByID(ctx, glbID)
		if err != nil {
			logger.Debugf("Error getting global load balancer %s when wait active: %v", glbID, err)
			return false, err
		}
		lastStatus = lb.Status
		if strings.ToUpper(lb.Status) == consts.ACTIVE_LOADBALANCER_STATUS {
			logger.Debugf("Global load balancer %s is ready", glbID)
			resultGLB = lb
			return true, nil
		}
		if strings.ToUpper(lb.Status) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Debugf("Global load balancer %s is in error status", glbID)
			resultGLB = lb
			return true, domain.ErrorLoadBalancerStatusError
		}

		logger.Debugf("Global load balancer %s is `%s`, waiting...", glbID, lb.Status)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for global load balancer %s to become active, last status %q", glbID, lastStatus)
	}

	return resultGLB, err
}

// --------------------------- Global Pool ---------------------------

func (m *vngCloudRepository) ListGlobalPools(ctx context.Context, glbID string) (*entityv2.ListGlobalPools, error) {
	logger := contexts.NewContext(ctx).Log()
	pools, sdkErr := m.client.GLBGateway().V1().GLBService().ListGlobalPools(global.NewListGlobalPoolsRequest(glbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListGlobalPools: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return pools, nil
}

func (m *vngCloudRepository) CreateGlobalPool(ctx context.Context, glbID string, opt global.ICreateGlobalPoolRequest) (*entityv2.GlobalPool, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create global pool of global load balancer %s", glbID)
	pool, sdkErr := m.client.GLBGateway().V1().GLBService().CreateGlobalPool(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateGlobalPool: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return pool, nil
}

func (m *vngCloudRepository) DeleteGlobalPool(ctx context.Context, glbID, poolID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete global pool %s of global load balancer %s", poolID, glbID)
	err := m.client.GLBGateway().V1().GLBService().DeleteGlobalPool(global.NewDeleteGlobalPoolRequest(glbID, poolID).AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("DeleteGlobalPool: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}

func (m *vngCloudRepository) UpdateGlobalPool(ctx context.Context, glbID, poolID string, opt global.IUpdateGlobalPoolRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request update global pool %s of global load balancer %s", poolID, glbID)
	_, err := m.client.GLBGateway().V1().GLBService().UpdateGlobalPool(opt.AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("UpdateGlobalPool: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}

func (m *vngCloudRepository) ListGlobalPoolMembers(ctx context.Context, glbID, poolID string) (*entityv2.ListGlobalPoolMembers, error) {
	logger := contexts.NewContext(ctx).Log()
	poolMembers, sdkErr := m.client.GLBGateway().V1().GLBService().ListGlobalPoolMembers(global.NewListGlobalPoolMembersRequest(glbID, poolID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListGlobalPoolMembers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return poolMembers, nil
}

func (m *vngCloudRepository) PatchGlobalPoolMembers(ctx context.Context, glbID, poolID string, opt global.IPatchGlobalPoolMembersRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request patch global pool members of pool %s of global load balancer %s", poolID, glbID)
	err := m.client.GLBGateway().V1().GLBService().PatchGlobalPoolMembers(opt.WithPoolId(poolID).WithLoadBalancerId(glbID).AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("PatchGlobalPoolMembers: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}

// --------------------------- Global Listener ---------------------------

func (m *vngCloudRepository) ListGlobalListeners(ctx context.Context, glbID string) (*entityv2.ListGlobalListeners, error) {
	logger := contexts.NewContext(ctx).Log()
	listeners, sdkErr := m.client.GLBGateway().V1().GLBService().ListGlobalListeners(global.NewListGlobalListenersRequest(glbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListGlobalListeners: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return listeners, nil
}

func (m *vngCloudRepository) GetGlobalListener(ctx context.Context, glbID, listenerID string) (*entityv2.GlobalListener, error) {
	logger := contexts.NewContext(ctx).Log()
	listener, sdkErr := m.client.GLBGateway().V1().GLBService().GetGlobalListener(global.NewGetGlobalListenerRequest(glbID, listenerID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("GetGlobalListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return listener, nil
}

func (m *vngCloudRepository) CreateGlobalListener(ctx context.Context, glbID string, opt global.ICreateGlobalListenerRequest) (*entityv2.GlobalListener, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create global listener of global load balancer %s", glbID)
	listener, sdkErr := m.client.GLBGateway().V1().GLBService().CreateGlobalListener(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateGlobalListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return listener, nil
}

func (m *vngCloudRepository) DeleteGlobalListener(ctx context.Context, glbID, listenerID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete global listener %s of global load balancer %s", listenerID, glbID)
	err := m.client.GLBGateway().V1().GLBService().DeleteGlobalListener(global.NewDeleteGlobalListenerRequest(glbID, listenerID).AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("DeleteGlobalListener: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}

func (m *vngCloudRepository) UpdateGlobalListener(ctx context.Context, glbID, listenerID string, opt global.IUpdateGlobalListenerRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request update global listener %s of global load balancer %s", listenerID, glbID)
	_, err := m.client.GLBGateway().V1().GLBService().UpdateGlobalListener(opt.AddUserAgent(m.userAgent))
	if err != nil {
		logger.Debug("UpdateGlobalListener: ", err, ", params: ", err.GetListParameters())
		return domain.SDKError(err)
	}
	return nil
}
