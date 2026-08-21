package vngcloud_repo

import (
	"context"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/inter"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

// --------------------------- Load Balancer ---------------------------

func (m *vngCloudRepository) ListLoadBalancers(ctx context.Context, tags []string) (*entityv2.ListLoadBalancers, error) {
	logger := contexts.NewContext(ctx).Log()
	lbs, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListLoadBalancers(loadbalancerv2.NewListLoadBalancersRequest(defaultOffset, defaultPageSize).WithTags(tags...).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListLoadBalancers: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return lbs, nil
}
func (m *vngCloudRepository) GetLoadBalancerByID(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	lb, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().GetLoadBalancerById(loadbalancerv2.NewGetLoadBalancerByIdRequest(lbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("GetLoadBalancerByID: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return lb, nil
}

func (m *vngCloudRepository) GetLoadBalancerByName(ctx context.Context, name string) (*entityv2.LoadBalancer, error) {
	allLBs, err := m.ListLoadBalancers(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, lb := range allLBs.Items {
		if lb.Name == name {
			return lb, nil
		}
	}
	return nil, nil
}

func (m *vngCloudRepository) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create load balancer %v", lbOptions.ToMap()["name"])
	newLB, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateLoadBalancer(lbOptions.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return newLB, nil
}
func (m *vngCloudRepository) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete load balancer %s", lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeleteLoadBalancerById(loadbalancerv2.NewDeleteLoadBalancerByIdRequest(lbID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("DeleteLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}
func (m *vngCloudRepository) ResizeLoadBalancer(ctx context.Context, lbID, packageID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request resize load balancer %s to package %s", lbID, packageID)

	opt := loadbalancerv2.NewResizeLoadBalancerRequest(lbID, packageID)
	_, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ResizeLoadBalancer(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ResizeLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}
func (m *vngCloudRepository) WaitForLBActive(ctx context.Context, lbID string) (*entityv2.LoadBalancer, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Waiting for load balancer %s to be ready", lbID)
	var resultLb *entityv2.LoadBalancer
	var lastStatus string

	err := wait.ExponentialBackoff(wait.Backoff{
		Duration: 5 * time.Second,
		Factor:   1.2,
		Steps:    30,
	}, func() (done bool, err error) {
		lb, err := m.GetLoadBalancerByID(ctx, lbID)
		if err != nil {
			logger.Debugf("Error getting load balancer %s when wait active: %v", lbID, err)
			return false, err
		}
		lastStatus = lb.DisplayStatus
		if strings.ToUpper(lb.DisplayStatus) == consts.ACTIVE_LOADBALANCER_STATUS &&
			strings.ToUpper(lb.ProgressStatus) == consts.CREATED_LOADBALANCER_STATUS {
			logger.Debugf("Load balancer %s is ready", lbID)
			resultLb = lb
			return true, nil
		}
		if strings.ToUpper(lb.DisplayStatus) == consts.ERROR_LOADBALANCER_STATUS {
			logger.Debugf("Load balancer %s is in error status", lbID)
			resultLb = lb
			return true, domain.ErrorLoadBalancerStatusError
		}

		logger.Debugf("Load balancer %s is not ready yet, waiting...", lbID)
		return false, nil
	})

	if wait.Interrupted(err) {
		logger.Errorf("timeout waiting for load balancer %s to become active, last status %q", lbID, lastStatus)
	}

	return resultLb, err
}

func (m *vngCloudRepository) ListLoadBalancerPackageByZone(ctx context.Context, zone common.Zone) (*entityv2.ListLoadBalancerPackages, error) {
	logger := contexts.NewContext(ctx).Log()

	opt := loadbalancerv2.NewListLoadBalancerPackagesRequest()
	packages, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListLoadBalancerPackages(opt.AddUserAgent(m.userAgent).WithZoneId(zone))
	if sdkErr != nil {
		logger.Debug("ListLoadBalancerPackageByZone: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return packages, nil
}

func (m *vngCloudRepository) CreateInterLoadBalancer(ctx context.Context, lbOptions inter.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	if m.superClient == nil {
		return nil, domain.ErrorSuperClientNotInitialized
	}

	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create INTERVPC load balancer %v", lbOptions.ToMap()["name"])
	newLB, sdkErr := m.superClient.VLBGateway().Internal().LoadBalancerService().
		CreateLoadBalancer(lbOptions.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateInterLoadBalancer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return newLB, nil
}
