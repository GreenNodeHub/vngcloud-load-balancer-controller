package ingress_uc

import (
	"context"

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
