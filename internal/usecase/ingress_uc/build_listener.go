package ingress_uc

import (
	"context"
	"sort"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

func (t *defaultModelBuildTask) buildListeners(ctx context.Context, isHttps bool) (*v1alpha1.Listener, error) {
	defaultIdleTimeoutClient := t.buildIdleTimeoutClient(ctx)
	defaultIdleTimeoutMember := t.buildIdleTimeoutMember(ctx)
	defaultIdleTimeoutConnection := t.buildIdleTimeoutConnection(ctx)
	defaultAllowedCidrs := t.buildInboundCIDRs(ctx)

	opt := v1alpha1.Listener{
		Name:              domain.DEFAULT_HTTP_LISTENER_NAME,
		Protocol:          loadbalancerv2.ListenerProtocolHTTP,
		ProtocolPort:      80,
		DefaultPoolName:   nil, // set later
		TimeoutClient:     defaultIdleTimeoutClient,
		TimeoutMember:     defaultIdleTimeoutMember,
		TimeoutConnection: defaultIdleTimeoutConnection,
		AllowedCidrs:      defaultAllowedCidrs,

		CertificateDefault:     nil,
		CertificateAuthorities: nil,
		ClientCertificateId:    nil,
		InsertHeaders:          t.buildHttpInsertHeaders(ctx),
	}

	if isHttps {
		opt.Name = domain.DEFAULT_HTTPS_LISTENER_NAME
		opt.Protocol = loadbalancerv2.ListenerProtocolHTTPS
		opt.ProtocolPort = 443

		opt.CertificateDefault, opt.CertificateAuthorities = t.buildHttpsListenerCertificates(ctx)
		opt.ClientCertificateId = t.buildClientCertificateId(ctx)
		opt.InsertHeaders = t.buildHttpsInsertHeaders(ctx)
	}

	return &opt, nil
}

func (t *defaultModelBuildTask) buildHttpsListenerCertificates(ctx context.Context) (*v1alpha1.ListenerCertificate, []v1alpha1.ListenerCertificate) {
	// try to get certificate IDs from annotation
	if certIDs := t.buildAnnotationCertificateIds(ctx); len(certIDs) > 0 {
		certificates := make([]v1alpha1.ListenerCertificate, 0, len(certIDs))
		for _, certID := range certIDs {
			certificates = append(certificates, v1alpha1.ListenerCertificate{
				Id: &certID,
			})
		}
		// NOTE: Do NOT sort certificates from annotation - order is intentional!
		// The first certificate is used as the default certificate,
		// the rest are used for SNI (Server Name Indication).
		// Sorting would make the default certificate selection unpredictable.
		return &certificates[0], certificates[1:]
	}

	// otherwise, add secret from TLS secrets
	certificates := make([]v1alpha1.ListenerCertificate, 0)
	for _, tls := range t.ingress.Spec.TLS {
		if tls.SecretName != "" {
			certificates = append(certificates, v1alpha1.ListenerCertificate{
				SecretName: ptr.To(tls.SecretName),
			})
		}
	}

	// Check if we have at least one certificate
	if len(certificates) == 0 {
		return nil, nil
	}

	// Sort by SecretName to ensure deterministic ordering
	sort.Slice(certificates, func(i, j int) bool {
		return *certificates[i].SecretName < *certificates[j].SecretName
	})

	return &certificates[0], certificates[1:]
}

func (t *defaultModelBuildTask) buildClientCertificateId(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixClientCertificateID, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildHttpInsertHeaders(ctx context.Context) []v1alpha1.InsertHeader {
	option := t.buildAnnotationInsertHeaders(ctx)
	headers := make([]v1alpha1.InsertHeader, 0)
	for k, v := range option.Http {
		headers = append(headers, v1alpha1.InsertHeader{
			HeaderName:  k,
			HeaderValue: v,
		})
	}
	// Sort by HeaderName to ensure deterministic ordering (map iteration is non-deterministic)
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].HeaderName < headers[j].HeaderName
	})
	return headers
}

func (t *defaultModelBuildTask) buildHttpsInsertHeaders(ctx context.Context) []v1alpha1.InsertHeader {
	option := t.buildAnnotationInsertHeaders(ctx)
	headers := make([]v1alpha1.InsertHeader, 0)
	for k, v := range option.Https {
		headers = append(headers, v1alpha1.InsertHeader{
			HeaderName:  k,
			HeaderValue: v,
		})
	}
	// Sort by HeaderName to ensure deterministic ordering (map iteration is non-deterministic)
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].HeaderName < headers[j].HeaderName
	})
	return headers
}

type insertHeadersConfig struct {
	Http  map[string]string `json:"http"`
	Https map[string]string `json:"https"`
}

func (t *defaultModelBuildTask) buildAnnotationInsertHeaders(_ context.Context) insertHeadersConfig {
	option := insertHeadersConfig{
		Http: map[string]string{
			"X-Forwarded-For":   "true",
			"X-Forwarded-Proto": "true",
			"X-Forwarded-Port":  "true",
		},
		Https: map[string]string{
			"X-Forwarded-For":   "true",
			"X-Forwarded-Proto": "true",
			"X-Forwarded-Port":  "true",
		},
	}
	exist, err := t.annotationParser.ParseJSONAnnotation(annotations.SuffixInsertHeaders, &option, t.ingress.Annotations)
	if !exist {
		return option
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a JSON object, using default value %v",
			annotations.SuffixInsertHeaders, option)
		return option
	}
	if option.Http == nil {
		option.Http = make(map[string]string)
	}
	if option.Https == nil {
		option.Https = make(map[string]string)
	}
	return option
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpMethod(_ context.Context) *loadbalancerv2.HealthCheckMethod {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpMethod, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	switch option {
	case
		string(loadbalancerv2.HealthCheckMethodGET),
		string(loadbalancerv2.HealthCheckMethodPUT),
		string(loadbalancerv2.HealthCheckMethodPOST):
		return ptr.To(loadbalancerv2.HealthCheckMethod(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\", \"%s\" or \"%s\"",
			annotations.SuffixHealthcheckHttpMethod,
			loadbalancerv2.HealthCheckMethodGET,
			loadbalancerv2.HealthCheckMethodPUT,
			loadbalancerv2.HealthCheckMethodPOST)
		return nil
	}
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckPath(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckPath, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationSuccessCodes(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixSuccessCodes, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpVersion(_ context.Context) *loadbalancerv2.HealthCheckHttpVersion {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpVersion, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	switch option {
	case string(loadbalancerv2.HealthCheckHttpVersionHttp1),
		string(loadbalancerv2.HealthCheckHttpVersionHttp1Minor1):
		return ptr.To(loadbalancerv2.HealthCheckHttpVersion(option))
	default:
		t.logger.Warnf("Invalid annotation \"%s\" value, must be \"%s\" or \"%s\"",
			annotations.SuffixHealthcheckHttpVersion,
			loadbalancerv2.HealthCheckHttpVersionHttp1,
			loadbalancerv2.HealthCheckHttpVersionHttp1Minor1)
	}
	return nil
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpDomainName(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpDomainName, &option, t.ingress.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationAutoReorderPolicies(_ context.Context) *bool {
	option := false
	exists, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixAutoReorderPolicies, &option, t.ingress.Annotations)
	if !exists {
		return nil
	}
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a boolean.",
			annotations.SuffixAutoReorderPolicies)
		return nil
	}
	return &option
}

// buildAutoAddPolicyPosition sets the policy positions automatically based on their priorities.
func (t *defaultModelBuildTask) buildAutoAddPolicyPosition(_ context.Context, listener *v1alpha1.Listener) error {
	if listener == nil || len(listener.Policies) == 0 {
		return nil
	}

	type kv struct {
		key      string
		priority int
	}
	ss := make([]kv, 0, len(listener.Policies))
	for _, policy := range listener.Policies {
		ss = append(ss, kv{key: policy.Name, priority: t.getPolicyPriority(&policy)})
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].priority > ss[j].priority
	})

	// update the new policy positions
	for i, policyName := range ss {
		for j, policy := range listener.Policies {
			if policy.Name == policyName.key {
				listener.Policies[j].Position = ptr.To(int32(i + 1))
				break
			}
		}
	}

	return nil
}

func (t *defaultModelBuildTask) getPolicyPriority(policy *v1alpha1.Policy) int {
	totalPriority := 0
	for _, rule := range policy.L7Rules {
		totalPriority += t.getRulePriority(&rule)
	}
	return totalPriority
}

func (t *defaultModelBuildTask) getRulePriority(rule *v1alpha1.L7Rule) int {
	compareTypeValue := 1
	switch rule.CompareType {
	case loadbalancerv2.PolicyCompareTypeREGEX:
		compareTypeValue = 10
	case loadbalancerv2.PolicyCompareTypeCONTAINS:
		compareTypeValue = 100
	case loadbalancerv2.PolicyCompareTypeENDSWITH:
		compareTypeValue = 1000
	case loadbalancerv2.PolicyCompareTypeSTARTSWITH:
		compareTypeValue = 10000
	case loadbalancerv2.PolicyCompareTypeEQUALS:
		compareTypeValue = 100000
	}

	ruleTypeValue := 1
	switch rule.RuleType {
	case loadbalancerv2.PolicyRuleTypeHOSTNAME:
		ruleTypeValue = 100
	case loadbalancerv2.PolicyRuleTypePATH:
		ruleTypeValue = 1
	}

	return compareTypeValue * ruleTypeValue * len(rule.RuleValue)
}
