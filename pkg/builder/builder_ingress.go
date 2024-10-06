package builder

import (
	"context"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// depend on ingress provided, build the LoadbalancerBuilder
func NewLoadBalancerBuilderByIngress(
	ctx context.Context,
	ingress *networkingv1.Ingress,
	annotationParser annotations.Parser,
	client client.Client,
	networkID, subnetID, subnetCIDR string,
	clusterID string,
	nodes []*corev1.Node,
) (LoadbalancerBuilder, error) {
	model := &lbBuilder{
		resourceType:      "ingress",
		resourceName:      "",
		resourceNamespace: "",

		secGroupRuleBuilders: make([]*secGroupRuleBuilderType, 0),
		poolBuilders:         make([]*poolBuilderType, 0),

		annotationParser: annotationParser,
		context:          ctx,
		client:           client,
		logger:           contexts.NewContext(ctx).Log(),

		networkID:  networkID,
		subnetID:   subnetID,
		subnetCIDR: subnetCIDR,
		clusterID:  clusterID,

		isIgnored: false,

		loadBalancerID:             "",
		loadBalancerName:           "",
		LoadBalancerType:           loadbalancerv2.LoadBalancerTypeLayer7,
		packageID:                  consts.DEFAULT_L7_PACKAGE_ID,
		Scheme:                     loadbalancerv2.InternetLoadBalancerScheme,
		IdleTimeoutClient:          50,
		IdleTimeoutMember:          50,
		IdleTimeoutConnection:      5,
		InboundCIDRs:               []string{"0.0.0.0/0"},
		HealthcheckProtocol:        loadbalancerv2.HealthCheckProtocolTCP,
		HealthcheckHttpMethod:      loadbalancerv2.HealthCheckMethodGET,
		HealthcheckPath:            "/",
		SuccessCodes:               "200",
		HealthcheckHttpVersion:     loadbalancerv2.HealthCheckHttpVersionHttp1,
		HealthcheckHttpDomainName:  "",
		PoolAlgorithm:              loadbalancerv2.PoolAlgorithmRoundRobin,
		HealthyThresholdCount:      3,
		UnhealthyThresholdCount:    3,
		HealthcheckTimeoutSeconds:  5,
		HealthcheckIntervalSeconds: 30,
		HealthcheckPort:            0,
		Tags:                       map[string]string{},
		TargetNodeLabels:           map[string]string{},
		IsAutoCreateSecurityGroup:  false,
		SecurityGroups:             []string{},
		EnableProxyProtocol:        []string{},
		EnableAutoscale:            false,
	}
	if ingress == nil {
		return model, nil
	}

	model.resourceName = ingress.Name
	model.resourceNamespace = ingress.Namespace

	model.parseAnnotation(ingress.Annotations)

	model.buildIngress(ingress, nodes)

	return model, nil
}

func (l *lbBuilder) buildIngress(ingress *networkingv1.Ingress, nodes []*corev1.Node) error {
	return nil
}
