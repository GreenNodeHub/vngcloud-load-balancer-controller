package service_uc

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type defaultModelBuildTask struct {
	// clusterName      string
	// vpcID            string
	annotationParser annotations.Parser
	serviceUtils     service.ServiceUtils

	// subnetsResolver            networking.SubnetsResolver
	// vpcInfoProvider            networking.VPCInfoProvider
	// backendSGProvider          networking.BackendSGProvider
	// sgResolver                 networking.SecurityGroupResolver
	// trackingProvider           tracking.Provider
	// elbv2TaggingManager        elbv2deploy.TaggingManager
	// featureGates               config.FeatureGates
	// enableManageBackendSGRules bool
	// ec2Client                  services.EC2
	// logger                     logr.Logger
	// metricsCollector           lbcmetrics.MetricCollector

	service          *corev1.Service
	vlbConfig        *v1alpha1.VngcloudLoadBalancerConfig
	vngcloudRepo     repository.IVngCloudRepository
	k8sRepo          repository.IK8sRepository
	nameHelper       utils.NameHelper
	cniMode          utils.CNIType
	endpointResolver utils.EndpointResolver

	// stack                    core.Stack
	// loadBalancer             *elbv2model.LoadBalancer
	// tgByResID                map[string]*elbv2model.TargetGroup
	// ec2Subnets               []ec2types.Subnet
	// enableBackendSG          bool
	// disableRestrictedSGRules bool
	// backendSGIDToken         core.StringToken
	// backendSGAllocated       bool
	// preserveClientIP         bool

	// fetchExistingLoadBalancerOnce sync.Once
	// existingLoadBalancer          *elbv2deploy.LoadBalancerWithTags

	// defaultPackageId string
	// defaultTags                          map[string]string
	// externalManagedTags                  sets.String
	// defaultSSLPolicy                     string
	// defaultAccessLogS3Enabled            bool
	// defaultAccessLogsS3Bucket            string
	// defaultAccessLogsS3Prefix            string
	// defaultIPAddressType                 elbv2model.IPAddressType
	// defaultLoadBalancingCrossZoneEnabled bool
	// defaultProxyProtocolV2Enabled        bool
	// defaultLoadBalancerScheme            elbv2model.LoadBalancerScheme
	// defaultHealthCheckProtocol           elbv2model.Protocol
	// defaultHealthCheckPort               string
	// defaultHealthCheckPath               string
	// defaultHealthCheckInterval           int32
	// defaultHealthCheckTimeout            int32
	// defaultHealthCheckHealthyThreshold   int32
	// defaultHealthCheckUnhealthyThreshold int32
	// defaultHealthCheckMatcherHTTPCode    string
	// defaultDeletionProtectionEnabled     bool
	// defaultIPv4SourceRanges              []string
	// defaultIPv6SourceRanges              []string

	// // Default health check settings for NLB instance mode with spec.ExternalTrafficPolicy set to Local
	// defaultHealthCheckProtocolForInstanceModeLocal           elbv2model.Protocol
	// defaultHealthCheckPortForInstanceModeLocal               string
	// defaultHealthCheckPathForInstanceModeLocal               string
	// defaultHealthCheckIntervalForInstanceModeLocal           int32
	// defaultHealthCheckTimeoutForInstanceModeLocal            int32
	// defaultHealthCheckHealthyThresholdForInstanceModeLocal   int32
	// defaultHealthCheckUnhealthyThresholdForInstanceModeLocal int32

	// enableTCPUDPSupport bool

	networkId  string
	subnetId   string
	subnetCIDR string
	zone       common.Zone
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()
	if !t.serviceUtils.IsServiceSupported(t.service) {
		if t.serviceUtils.IsServicePendingFinalization(t.service) {
			logger.Info("Service is not supported but pending finalization, running delete flow TODO")
		}
		return nil
	}
	err := t.buildModel(ctx)
	return err
}

func (t *defaultModelBuildTask) buildModel(ctx context.Context) error {
	if t.vlbConfig == nil {
		if err := t.buildSubnetAndZone(ctx); err != nil {
			return err
		}

		// build default VLBC
		t.vlbConfig = &v1alpha1.VngcloudLoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.GenerateLBConfigName("svc", t.service.Name),
				Namespace: t.service.Namespace,
				Labels:    map[string]string{
					// "vks": "TODO",
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: t.service.APIVersion,
						Kind:       t.service.Kind,
						Name:       t.service.Name,
						UID:        t.service.UID,
						// TODO
					},
				},
			},
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
				Type:     v2.LoadBalancerTypeLayer4,
				SubnetID: t.subnetId,
			},
		}
	}

	t.vlbConfig.Spec.PackageID = t.buildPackageId(ctx)
	t.vlbConfig.Spec.Scheme = t.buildScheme(ctx)
	t.vlbConfig.Spec.EnableAutoscale = t.buildAutoscale(ctx)
	t.vlbConfig.Spec.Tags = t.buildTags(ctx)
	t.vlbConfig.Spec.TargetNodeLabels = t.buildTargetNodeLabels(ctx)
	t.vlbConfig.Spec.IsPoc = t.buildIsPoc(ctx)

	if t.vlbConfig.Spec.LoadBalancerName == "" {
		t.vlbConfig.Spec.LoadBalancerName = t.buildLoadBalancerName(ctx)
	}
	if t.vlbConfig.Spec.SubnetID == "" {
		t.vlbConfig.Spec.SubnetID = t.subnetId
	}

	if err := t.buildPoolsAndListeners(ctx); err != nil {
		return err
	}
	if isAutoCreateSecGroup, secgroupIds := t.buildIsAutoCreateSecGroup(ctx); !isAutoCreateSecGroup {
		t.vlbConfig.Spec.AutoManageSecurityGroupRules = nil
		t.vlbConfig.Spec.AttachSecurityGroupsToNodes = secgroupIds
	} else {
		secgroupRules, err := t.buildDefaultSecurityGroupRule(ctx)
		if err != nil {
			return err
		}
		t.vlbConfig.Spec.AutoManageSecurityGroupRules = secgroupRules
		t.vlbConfig.Spec.AttachSecurityGroupsToNodes = nil
	}

	return nil
}

func (t *defaultModelBuildTask) buildPackageId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPackageID, &option, t.service.Annotations)
	if option == "" {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildScheme(_ context.Context) *v2.LoadBalancerScheme {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixScheme, &option, t.service.Annotations)
	switch option {
	case "internal":
		value := v2.InternalLoadBalancerScheme
		return &value
	case "internet":
		value := v2.InternetLoadBalancerScheme
		return &value
	default:
		return nil
	}
}

func (t *defaultModelBuildTask) buildSubnetAndZone(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()

	// try to get zone, subnet from prefer subnet id annotation
	var preferSubnetId string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferSubnetID, &preferSubnetId, t.service.Annotations)
	if preferSubnetId != "" {
		if preferSubnetId == t.subnetId {
			return nil
		}
		subnet, err := t.vngcloudRepo.GetSubnetByID(ctx, t.networkId, preferSubnetId)
		if err != nil || subnet == nil {
			logger.Warnf("Failed to get subnet: %s. Fall to prefer zone, then use default zone.", err)
			return nil
		}
		t.subnetId = subnet.Id
		t.subnetCIDR = subnet.Cidr
		t.zone = common.Zone(subnet.ZoneID)
	}

	// try to get zone, subnet from prefer zone id annotation
	var preferZoneId string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferZoneID, &preferZoneId, t.service.Annotations)
	if preferZoneId != "" {
		if common.Zone(preferZoneId) == t.zone {
			return nil
		}

		nodes := &corev1.NodeList{}
		err := t.k8sRepo.ListNode(ctx, nodes)
		if err != nil {
			logger.Warnf("Failed to list nodes: %s. Fall to default zone.", err)
			return nil
		}

		providerIds := utils.GetListProviderIdFromNodeList(nodes)
		for _, providerId := range providerIds {
			_zone, _networkID, _subnetID, _subnetCIDR, err := t.vngcloudRepo.GetServerNetworkInfo(ctx, providerId)
			if err != nil {
				continue
			}
			if string(_zone) == preferZoneId {
				t.zone = _zone
				t.networkId = _networkID
				t.subnetId = _subnetID
				t.subnetCIDR = _subnetCIDR
				return nil
			}
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context) string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerName, &option, t.service.Annotations)
	if option != "" {
		return option
	}
	return t.nameHelper.GetLoadBalancerDefaultName()
}

func (t *defaultModelBuildTask) buildAutoscale(ctx context.Context) *bool {
	logger := contexts.NewContext(ctx).Log()
	var autoscale bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableAutoscale, &autoscale, t.service.Annotations)
	if err != nil {
		logger.Warnf("Failed to parse autoscale annotation: %s", err)
		return nil
	}
	if !isExist {
		return nil
	}
	return &autoscale
}

func (t *defaultModelBuildTask) buildTags(ctx context.Context) map[string]string {
	logger := contexts.NewContext(ctx).Log()
	option := make(map[string]string)
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTags, &option, t.service.Annotations)
	if err != nil {
		logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTags, nil)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildTargetNodeLabels(ctx context.Context) map[string]string {
	logger := contexts.NewContext(ctx).Log()
	option := make(map[string]string)
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTargetNodeLabels, &option, t.service.Annotations)
	if err != nil {
		logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTargetNodeLabels, nil)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildIsPoc(ctx context.Context) *bool {
	logger := contexts.NewContext(ctx).Log()
	var isPoc bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC, &isPoc, t.service.Annotations)
	if err != nil {
		logger.Warnf("Failed to parse isPOC annotation: %s", err)
		return nil
	}
	if !isExist {
		// try deprecated annotation
		isExist, err = t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC2, &isPoc, t.service.Annotations)
		if err != nil {
			logger.Warnf("Failed to parse is-poc annotation: %s", err)
			return nil
		}
		if !isExist {
			return nil
		}
	}
	return &isPoc
}

// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
