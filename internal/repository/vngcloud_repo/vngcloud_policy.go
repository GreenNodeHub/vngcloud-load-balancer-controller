package vngcloud_repo

import (
	"context"
	"strings"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// --------------------------- Policy ---------------------------

//	func (m *vngCloudRepository) GetPolicyByName(ctx context.Context,lbID, listenerID, name string) (*objects.Policy, error) {
//		logger.Error("not implemented yet")
//		return nil, ErrorNotImplemented
//	}
func (m *vngCloudRepository) CreatePolicy(ctx context.Context, lbID, listenerID string, opt loadbalancerv2.ICreatePolicyRequest) (*entityv2.Policy, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create policy of listener %s of load balancer %s", listenerID, lbID)
	policy, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().CreatePolicy(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreatePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return policy, nil
}
func (m *vngCloudRepository) ListPolicyOfListener(ctx context.Context, lbID, listenerID string) (*entityv2.ListPolicies, error) {
	logger := contexts.NewContext(ctx).Log()
	listPolicies, sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ListPolicies(loadbalancerv2.NewListPoliciesRequest(lbID, listenerID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListPolicyOfListener: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return listPolicies, nil
}

//	func (m *vngCloudRepository) GetPolicyByID(ctx context.Context,policyID string) (*objects.Policy, error) {
//		logger.Error("not implemented yet")
//		return nil, ErrorNotImplemented
//	}
func (m *vngCloudRepository) UpdatePolicy(ctx context.Context, lbID, listenerID, policyID string, opt loadbalancerv2.IUpdatePolicyRequest) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request update policy %s of listener %s of load balancer %s", policyID, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().UpdatePolicy(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("UpdatePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}
func (m *vngCloudRepository) DeletePolicy(ctx context.Context, lbID, listenerID, policyID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete policy %s of listener %s of load balancer %s", policyID, listenerID, lbID)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().DeletePolicyById(loadbalancerv2.NewDeletePolicyByIdRequest(lbID, listenerID, policyID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("DeletePolicy: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}

func (m *vngCloudRepository) ReorderPolicies(ctx context.Context, lbID, listenerID string, policyIDs []string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request reorder %d policies of listener %s of load balancer %s", len(policyIDs), listenerID, lbID)
	logger.Debugf("Reorder policy IDs: [%s]", strings.Join(policyIDs, ","))
	opt := loadbalancerv2.NewReorderPoliciesRequest(lbID, listenerID).WithPoliciesOrder(policyIDs)
	sdkErr := m.client.VLBGateway().V2().LoadBalancerService().ReorderPolicies(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ReorderPolicies: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}
