package ingress_uc

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

func (t *defaultModelBuildTask) buildPolicyByPath(ctx context.Context, host, policyName string, path *networkingv1.HTTPIngressPath) (*v1alpha1.Policy, error) {
	// compare type
	var compareType loadbalancerv2.PolicyCompareType
	switch *path.PathType {
	case networkingv1.PathTypeExact:
		compareType = loadbalancerv2.PolicyCompareTypeEQUALS
	case networkingv1.PathTypePrefix:
		compareType = loadbalancerv2.PolicyCompareTypeSTARTSWITH
	case networkingv1.PathTypeImplementationSpecific:
		// incase not specify annotation to config, use as path type Exact
		compareType = loadbalancerv2.PolicyCompareTypeEQUALS
		res, err := t.buildPolicyByRegex(ctx, host, policyName, host, path)
		if err == nil || err != domain.ErrorNoImplementationSpecificConfigFound {
			return res, err
		}

	default:
		compareType = loadbalancerv2.PolicyCompareTypeEQUALS
	}

	// path rule
	l7Rules := []v1alpha1.L7Rule{
		{
			CompareType: compareType,
			RuleType:    loadbalancerv2.PolicyRuleTypePATH,
			RuleValue:   path.Path,
		},
	}

	if host != "" {
		l7Rules = append(l7Rules, v1alpha1.L7Rule{
			CompareType: loadbalancerv2.PolicyCompareTypeEQUALS,
			RuleType:    loadbalancerv2.PolicyRuleTypeHOSTNAME,
			RuleValue:   host,
		})
	}

	// build policy
	return &v1alpha1.Policy{
		Name:    policyName,
		Action:  loadbalancerv2.PolicyActionREDIRECTTOPOOL,
		L7Rules: l7Rules,
	}, nil
}

func (t *defaultModelBuildTask) buildPolicyByRegex(ctx context.Context, _, policyName, host string, path *networkingv1.HTTPIngressPath) (*v1alpha1.Policy, error) {
	for _, config := range t.buildImplementationSpecificConfigs(ctx) {
		if config.Path == path.Path && config.Host == host {
			t.logger.Debugf("Found implementation specific config for path %s: %v", path.Path, config)
			// validate rules
			for _, rule := range config.Rules {

				// validate rule type
				validRuleType := []string{
					string(loadbalancerv2.PolicyRuleTypePATH),
					string(loadbalancerv2.PolicyRuleTypeHOSTNAME),
				}
				if !slices.Contains(validRuleType, rule.RuleType) {
					return nil, errs.NewNoNeedRequeue(fmt.Sprintf("invalid \"type\": \"%s\", must be one of %s", rule.RuleType, validRuleType))
				}

				// validate compare type
				validCompareType := []string{
					string(loadbalancerv2.PolicyCompareTypeCONTAINS),
					string(loadbalancerv2.PolicyCompareTypeENDSWITH),
					string(loadbalancerv2.PolicyCompareTypeSTARTSWITH),
					string(loadbalancerv2.PolicyCompareTypeREGEX),
					string(loadbalancerv2.PolicyCompareTypeEQUALS),
				}
				if !slices.Contains(validCompareType, rule.Compare) {
					return nil, errs.NewNoNeedRequeue(fmt.Sprintf("invalid \"compare\": \"%s\", must be one of %s", rule.Compare, validCompareType))
				}
			}

			// validate action
			validAction := []string{
				string(loadbalancerv2.PolicyActionREDIRECTTOPOOL),
				string(loadbalancerv2.PolicyActionREDIRECTTOURL),
				string(loadbalancerv2.PolicyActionREJECT),
			}
			if !slices.Contains(validAction, config.Action.Action) {
				return nil, errs.NewNoNeedRequeue(fmt.Sprintf("invalid \"action\": \"%s\", must be one of %s", config.Action.Action, validAction))

			}

			// validate if action is redirect to url
			if config.Action.Action == string(loadbalancerv2.PolicyActionREDIRECTTOURL) {
				if _, err := url.ParseRequestURI(config.Action.RedirectURL); err != nil {
					return nil, errs.NewNoNeedRequeue(fmt.Sprintf("invalid \"redirectUrl\": \"%s\"", config.Action.RedirectURL))
				}
				if !slices.Contains([]int{301, 302}, config.Action.RedirectHTTPCode) {
					return nil, errs.NewNoNeedRequeue(fmt.Sprintf("invalid \"redirectHttpCode\": \"%d\", must be one of 301, 302", config.Action.RedirectHTTPCode))
				}
			} else {
				config.Action.RedirectURL = ""
				config.Action.RedirectHTTPCode = 301
			}

			l7Rule := make([]v1alpha1.L7Rule, 0)
			for _, rule := range config.Rules {
				// check if rule is duplicated
				for _, r := range l7Rule {
					if r.CompareType == loadbalancerv2.PolicyCompareType(rule.Compare) &&
						r.RuleType == loadbalancerv2.PolicyRuleType(rule.RuleType) &&
						r.RuleValue == rule.Value {
						return nil, errs.NewNoNeedRequeue(fmt.Sprintf("duplicated rule: %v", rule))
					}
				}
				l7Rule = append(l7Rule, v1alpha1.L7Rule{
					CompareType: loadbalancerv2.PolicyCompareType(rule.Compare),
					RuleType:    loadbalancerv2.PolicyRuleType(rule.RuleType),
					RuleValue:   rule.Value,
				})
			}

			return &v1alpha1.Policy{
				Name:             policyName,
				Action:           loadbalancerv2.PolicyAction(config.Action.Action),
				L7Rules:          l7Rule,
				RedirectUrl:      ptr.To(config.Action.RedirectURL),
				RedirectHttpCode: ptr.To(int32(config.Action.RedirectHTTPCode)),
				KeepQueryString:  ptr.To(config.Action.KeepQueryString),
			}, nil
		}
	}

	t.logger.Warnf("No implementation specific config found for host '%s' and path '%s'.", host, path.Path)

	return nil, domain.ErrorNoImplementationSpecificConfigFound
}

type rule struct {
	RuleType string `json:"type"`
	Compare  string `json:"compare"`
	Value    string `json:"value"`
}

type action struct {
	Action           string `json:"action"`
	RedirectURL      string `json:"redirectUrl"`
	RedirectHTTPCode int    `json:"redirectHttpCode"`
	KeepQueryString  bool   `json:"keepQueryString"`
}

type implementationSpecificConfig struct {
	Host   string `json:"host"`
	Path   string `json:"path"`
	Rules  []rule `json:"rules"`
	Action action `json:"action"`
}

func (t *defaultModelBuildTask) buildImplementationSpecificConfigs(_ context.Context) []implementationSpecificConfig {
	option := make([]implementationSpecificConfig, 0)
	exist, err := t.annotationParser.ParseJSONAnnotation(annotations.SuffixImplementationSpecificParams, &option, t.ingress.Annotations)
	if !exist {
		return option
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a JSON object.",
			annotations.SuffixImplementationSpecificParams)
		return option
	}
	return option
}
