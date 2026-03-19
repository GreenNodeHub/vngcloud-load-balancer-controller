package service_uc

import (
	"context"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpMethod(_ context.Context) *loadbalancerv2.HealthCheckMethod {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpMethod, &option, t.service.Annotations)
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
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckPath, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationSuccessCodes(_ context.Context) *string {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixSuccessCodes, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildAnnotationHealthcheckHttpVersion(_ context.Context) *loadbalancerv2.HealthCheckHttpVersion {
	option := ""
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpVersion, &option, t.service.Annotations)
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
	exist := t.annotationParser.ParseStringAnnotation(annotations.SuffixHealthcheckHttpDomainName, &option, t.service.Annotations)
	if !exist {
		return nil
	}
	return &option
}
