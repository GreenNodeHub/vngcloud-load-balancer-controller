package vngcloud_repo

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// --------------------------- Listener ---------------------------

func (m *vngCloudRepository) CreateListener(ctx context.Context, lbID string, opt loadbalancerv2.ICreateListenerRequest) (*entityv2.Listener, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request create listener of load balancer %s", domain.RequestIcon, lbID)
	listener, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreateListener(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - CreateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listener, nil
}
func (m *vngCloudRepository) ListListenerOfLB(ctx context.Context, lbID string) (*entityv2.ListListeners, error) {
	logger := contexts.NewContext(ctx).Log()
	opt := loadbalancerv2.NewListListenersByLoadBalancerIdRequest(lbID)
	listeners, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListListenersByLoadBalancerId(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - ListListenerOfLB: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, sdkErr.GetError()
	}
	return listeners, nil
}
func (m *vngCloudRepository) DeleteListener(ctx context.Context, lbID, listenerID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request delete listener %s of load balancer %s", domain.RequestIcon, listenerID, lbID)
	opt := loadbalancerv2.NewDeleteListenerByIdRequest(lbID, listenerID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeleteListenerById(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - DeleteListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
func (m *vngCloudRepository) UpdateListener(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.IUpdateListenerRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("%s Request update listener %s of load balancer %s", domain.RequestIcon, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdateListener(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Error("[ERROR] - UpdateListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return sdkErr.GetError()
	}
	return nil
}
