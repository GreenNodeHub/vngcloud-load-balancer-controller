package lbc_uc

import (
	"context"
	"fmt"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelDeployTask) deployPolicies(ctx context.Context, lbId, listenerId string, policiesSpec []v1alpha1.Policy, newCreatedPools []v1alpha1.CreatedPool) ([]v1alpha1.CreatedPolicy, error) {
	currentPolicies, err := t.vngcloudRepo.ListPolicyOfListener(ctx, lbId, listenerId)
	if err != nil {
		return nil, err
	}

	createdPolicies := make([]v1alpha1.CreatedPolicy, 0, len(policiesSpec))
	for _, policySpec := range policiesSpec {
		createdPolicy, err := t.deployPolicy(ctx, lbId, listenerId, policySpec, newCreatedPools, currentPolicies)
		if err != nil {
			return nil, err
		}
		createdPolicies = append(createdPolicies, *createdPolicy)
	}
	return createdPolicies, nil
}

func (t *defaultModelDeployTask) deployPolicy(ctx context.Context, lbId, listenerId string, policySpec v1alpha1.Policy, newCreatedPools []v1alpha1.CreatedPool, currentPolicies *entityv2.ListPolicies) (*v1alpha1.CreatedPolicy, error) {
	searchPolicyByName := func(name string) *entityv2.Policy {
		for _, p := range currentPolicies.Items {
			if p.Name == name {
				return p
			}
		}
		return nil
	}

	currentPolicy := searchPolicyByName(policySpec.Name)
	if currentPolicy == nil {
		createRequest, err := t.buildPolicyCreateRequest(ctx, lbId, listenerId, policySpec, newCreatedPools)
		if err != nil {
			return nil, err
		}
		newPolicy, err := t.vngcloudRepo.CreatePolicy(ctx, lbId, listenerId, createRequest)
		if err != nil {
			return nil, err
		}
		if err := t.statusAddPolicy(ctx, listenerId, newPolicy.UUID); err != nil {
			return nil, err
		}

		if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
			t.logger.Error("Failed to wait for loadbalancer active: ", err)
			return nil, err
		}
		return &v1alpha1.CreatedPolicy{Id: newPolicy.UUID}, nil
	} else {
		updateRequest, messages, err := t.buildPolicyUpdateRequest(ctx, lbId, listenerId, policySpec, newCreatedPools, currentPolicy)
		if err != nil {
			return nil, err
		}
		if updateRequest != nil {
			t.logger.Infof("Need update policy %s: %s.", policySpec.Name, strings.Join(messages, ", "))
			if err := t.vngcloudRepo.UpdatePolicy(ctx, lbId, listenerId, currentPolicy.UUID, updateRequest); err != nil {
				return nil, err
			}

			if _, err := t.vngcloudRepo.WaitForLBActive(ctx, lbId); err != nil {
				t.logger.Error("Failed to wait for loadbalancer active: ", err)
				return nil, err
			}
		}
	}
	return &v1alpha1.CreatedPolicy{Id: currentPolicy.UUID}, nil
}

func (t *defaultModelDeployTask) buildPolicyCreateRequest(_ context.Context, lbId, listenerId string, policySpec v1alpha1.Policy, newCreatedPools []v1alpha1.CreatedPool) (loadbalancerv2.ICreatePolicyRequest, error) {
	request := loadbalancerv2.NewCreatePolicyRequest(lbId, listenerId).
		WithAction(policySpec.Action).
		WithName(policySpec.Name).
		WithRules(
			func() []loadbalancerv2.L7RuleRequest {
				rules := make([]loadbalancerv2.L7RuleRequest, 0, len(policySpec.L7Rules))
				for _, ruleSpec := range policySpec.L7Rules {
					rules = append(rules, loadbalancerv2.L7RuleRequest{
						CompareType: loadbalancerv2.PolicyCompareType(ruleSpec.CompareType),
						RuleValue:   ruleSpec.RuleValue,
						RuleType:    loadbalancerv2.PolicyRuleType(ruleSpec.RuleType),
					})
				}
				return rules
			}()...,
		)

	// options for redirect to pool
	if policySpec.Action == loadbalancerv2.PolicyActionREDIRECTTOPOOL {
		if policySpec.RedirectPoolName != nil {
			findCreatedPoolByName := func(name string) *v1alpha1.CreatedPool {
				for _, p := range newCreatedPools {
					if p.Name == name {
						return &p
					}
				}
				return nil
			}
			if createdPool := findCreatedPoolByName(*policySpec.RedirectPoolName); createdPool != nil {
				request = request.WithRedirectPoolId(createdPool.Id)
			}
		}
	}

	// options for redirect to url
	if policySpec.Action == loadbalancerv2.PolicyActionREDIRECTTOURL {
		if policySpec.RedirectUrl != nil {
			request = request.WithRedirectURL(*policySpec.RedirectUrl)
		}
		if policySpec.RedirectHttpCode != nil {
			request = request.WithRedirectHTTPCode(int(*policySpec.RedirectHttpCode))
		}
		if policySpec.KeepQueryString != nil {
			request = request.WithKeepQueryString(*policySpec.KeepQueryString)
		}
	}
	return request, nil
}

func (t *defaultModelDeployTask) buildPolicyUpdateRequest(_ context.Context, lbId, listenerId string, policySpec v1alpha1.Policy, newCreatedPools []v1alpha1.CreatedPool, currentPolicy *entityv2.Policy) (loadbalancerv2.IUpdatePolicyRequest, []string, error) {
	isNeedUpdate := false
	message := make([]string, 0)
	updateOptions := &loadbalancerv2.UpdatePolicyRequest{
		LoadBalancerCommon: common.LoadBalancerCommon{
			LoadBalancerId: lbId,
		},
		ListenerCommon: common.ListenerCommon{
			ListenerId: listenerId,
		},
		PolicyCommon: common.PolicyCommon{
			PolicyId: currentPolicy.UUID,
		},
		Action:           loadbalancerv2.PolicyAction(currentPolicy.Action),
		KeepQueryString:  currentPolicy.KeepQueryString,
		RedirectPoolID:   currentPolicy.RedirectPoolID,
		RedirectURL:      currentPolicy.RedirectURL,
		RedirectHTTPCode: currentPolicy.RedirectHTTPCode,
		Rules: func() []loadbalancerv2.L7RuleRequest {
			rules := make([]loadbalancerv2.L7RuleRequest, 0, len(currentPolicy.L7Rules))
			for _, r := range currentPolicy.L7Rules {
				rules = append(rules, loadbalancerv2.L7RuleRequest{
					CompareType: loadbalancerv2.PolicyCompareType(r.CompareType),
					RuleValue:   r.RuleValue,
					RuleType:    loadbalancerv2.PolicyRuleType(r.RuleType),
				})
			}
			return rules
		}(),
	}

	if policySpec.Action != "" && string(policySpec.Action) != currentPolicy.Action {
		message = append(message, fmt.Sprintf("action (%s -> %s)", currentPolicy.Action, policySpec.Action))
		updateOptions.Action = loadbalancerv2.PolicyAction(policySpec.Action)
		isNeedUpdate = true
	}

	// options for redirect to pool
	if policySpec.Action == loadbalancerv2.PolicyActionREDIRECTTOPOOL {
		poolId := ""
		if policySpec.RedirectPoolName != nil {
			findCreatedPoolByName := func(name string) *v1alpha1.CreatedPool {
				for _, p := range newCreatedPools {
					if p.Name == name {
						return &p
					}
				}
				return nil
			}

			if createdPool := findCreatedPoolByName(*policySpec.RedirectPoolName); createdPool != nil {
				poolId = createdPool.Id
			}
		}

		if currentPolicy.RedirectPoolID != poolId {
			message = append(message, fmt.Sprintf("redirect pool id (%s -> %s)", currentPolicy.RedirectPoolID, poolId))
			updateOptions.RedirectPoolID = poolId
			isNeedUpdate = true
		}
	}

	// options for redirect to url
	if policySpec.Action == loadbalancerv2.PolicyActionREDIRECTTOURL {
		redirectUrl := ""
		if policySpec.RedirectUrl != nil {
			redirectUrl = *policySpec.RedirectUrl
		}
		if currentPolicy.RedirectURL != redirectUrl {
			message = append(message, fmt.Sprintf("redirect url (%s -> %s)", currentPolicy.RedirectURL, redirectUrl))
			updateOptions.RedirectURL = redirectUrl
			isNeedUpdate = true
		}

		redirectHttpCode := 0
		if policySpec.RedirectHttpCode != nil {
			redirectHttpCode = int(*policySpec.RedirectHttpCode)
		}
		if currentPolicy.RedirectHTTPCode != redirectHttpCode {
			message = append(message, fmt.Sprintf("redirect http code (%d -> %d)", currentPolicy.RedirectHTTPCode, redirectHttpCode))
			updateOptions.RedirectHTTPCode = redirectHttpCode
			isNeedUpdate = true
		}

		keepQueryString := false
		if policySpec.KeepQueryString != nil {
			keepQueryString = *policySpec.KeepQueryString
		}
		if currentPolicy.KeepQueryString != keepQueryString {
			message = append(message, fmt.Sprintf("keep query string (%t -> %t)", currentPolicy.KeepQueryString, keepQueryString))
			updateOptions.KeepQueryString = keepQueryString
			isNeedUpdate = true
		}
	}

	if l7Rules := t.compareL7Rules(policySpec.L7Rules, currentPolicy.L7Rules); l7Rules != nil {
		message = append(message, "rules updated")
		updateOptions.Rules = l7Rules
		isNeedUpdate = true
	}

	if !isNeedUpdate {
		return nil, nil, nil
	}
	return updateOptions, message, nil
}

func (t *defaultModelDeployTask) compareL7Rules(rulesSpec []v1alpha1.L7Rule, currentRules []*entityv2.L7Rule) []loadbalancerv2.L7RuleRequest {
	l7RuleRequests := make([]loadbalancerv2.L7RuleRequest, 0, len(rulesSpec))
	for _, ruleSpec := range rulesSpec {
		l7RuleRequests = append(l7RuleRequests, loadbalancerv2.L7RuleRequest{
			CompareType: loadbalancerv2.PolicyCompareType(ruleSpec.CompareType),
			RuleValue:   ruleSpec.RuleValue,
			RuleType:    loadbalancerv2.PolicyRuleType(ruleSpec.RuleType),
		})
	}

	if len(l7RuleRequests) != len(currentRules) {
		return l7RuleRequests
	}

	for _, r := range l7RuleRequests {
		found := false
		for _, cr := range currentRules {
			if string(r.CompareType) == cr.CompareType && r.RuleValue == cr.RuleValue && string(r.RuleType) == cr.RuleType {
				found = true
				break
			}
		}
		if !found {
			return l7RuleRequests
		}
	}

	return nil
}
