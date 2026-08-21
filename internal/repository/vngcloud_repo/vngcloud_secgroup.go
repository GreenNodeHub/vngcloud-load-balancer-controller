package vngcloud_repo

import (
	"context"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/sdk_error"
	computev2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/compute/v2"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// // --------------------------- Security Group ---------------------------

func (m *vngCloudRepository) ListSecurityGroups(ctx context.Context) (*entityv2.ListSecgroups, error) {
	logger := contexts.NewContext(ctx).Log()

	secgroups, sdkErr := m.client.VServerGateway().V2().NetworkService().ListSecgroup(networkv2.NewListSecgroupRequest().AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListSecurityGroups: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return secgroups, nil
}

func (m *vngCloudRepository) UpdateSecGroupsOfServer(ctx context.Context, instanceID string, secgroups []string) (*entityv2.Server, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request update security groups of server %s", instanceID)

	opt := computev2.NewUpdateServerSecgroupsRequest(instanceID, secgroups...)
	IsServerNotReady := func(err error) bool {
		return err != nil && strings.Contains(err.Error(), "Cannot change security group of server with status")
	}

	var sdkErr sdk_error.IError
	var server *entityv2.Server
	for i := 0; i < 3; i++ {
		server, sdkErr = m.client.VServerGateway().V2().ComputeService().UpdateServerSecgroupsByServerId(opt.AddUserAgent(m.userAgent))
		if sdkErr != nil {
			if IsServerNotReady(domain.SDKError(sdkErr)) {
				logger.Debugf("Server %s is not ready yet, waiting...", instanceID)
				time.Sleep(5 * time.Second)
				continue
			} else {
				logger.Debug("UpdateSecGroupsOfServer: ", sdkErr, ", params: ", sdkErr.GetListParameters())
				return nil, domain.SDKError(sdkErr)
			}
		} else {
			return server, nil
		}
	}
	return nil, domain.SDKError(sdkErr)
}

func (m *vngCloudRepository) GetSecurityGroup(ctx context.Context, secgroupID string) (*entityv2.Secgroup, error) {
	logger := contexts.NewContext(ctx).Log()

	secgroup, sdkErr := m.client.VServerGateway().V2().NetworkService().GetSecgroupById(networkv2.NewGetSecgroupByIdRequest(secgroupID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("GetSecurityGroup: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return secgroup, nil
}

func (m *vngCloudRepository) DeleteSecurityGroup(ctx context.Context, secgroupID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete security group %s", secgroupID)

	sdkErr := m.client.VServerGateway().V2().NetworkService().DeleteSecgroupById(networkv2.NewDeleteSecgroupByIdRequest(secgroupID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("DeleteSecurityGroup: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}

func (m *vngCloudRepository) CreateSecurityGroup(ctx context.Context, name string, description string) (*entityv2.Secgroup, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create security group %s", name)

	opt := networkv2.NewCreateSecgroupRequest(name, description)
	secgroup, sdkErr := m.client.VServerGateway().V2().NetworkService().CreateSecgroup(opt.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateSecurityGroup: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return secgroup, nil
}

func (m *vngCloudRepository) CreateSecurityGroupRule(ctx context.Context, secgroupID string, opts networkv2.ICreateSecgroupRuleRequest) (*entityv2.SecgroupRule, error) {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request create security group rule of security group %s", secgroupID)

	rule, sdkErr := m.client.VServerGateway().V2().NetworkService().CreateSecgroupRule(opts.AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("CreateSecurityGroupRule: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return rule, nil
}

func (m *vngCloudRepository) DeleteSecurityGroupRule(ctx context.Context, secgroupID string, ruleID string) error {
	logger := contexts.NewContext(ctx).Log()
	logger.Infof("Request delete security group rule %s of security group %s", ruleID, secgroupID)

	sdkErr := m.client.VServerGateway().V2().NetworkService().DeleteSecgroupRuleById(networkv2.NewDeleteSecgroupRuleByIdRequest(ruleID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("DeleteSecurityGroupRule: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return domain.SDKError(sdkErr)
	}
	return nil
}

func (m *vngCloudRepository) ListSecurityGroupRules(ctx context.Context, secgroupID string) (*entityv2.ListSecgroupRules, error) {
	logger := contexts.NewContext(ctx).Log()

	rules, sdkErr := m.client.VServerGateway().V2().NetworkService().ListSecgroupRulesBySecgroupId(networkv2.NewListSecgroupRulesBySecgroupIdRequest(secgroupID).AddUserAgent(m.userAgent))
	if sdkErr != nil {
		logger.Debug("ListSecurityGroupRules: ", sdkErr, ", params: ", sdkErr.GetListParameters())
		return nil, domain.SDKError(sdkErr)
	}
	return rules, nil
}
