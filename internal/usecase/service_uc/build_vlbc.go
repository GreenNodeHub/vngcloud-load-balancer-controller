package service_uc

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

	logger           *logrus.Entry
	service          *corev1.Service
	vlbConfig        *v1alpha1.VngcloudLoadBalancerConfig
	vngcloudRepo     repository.IVngCloudRepository
	k8sRepo          repository.IK8sRepository
	nameHelper       utils.NameHelper
	cniMode          utils.CNIType
	endpointResolver utils.EndpointResolver

	defaultZone       common.Zone
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if !t.serviceUtils.IsServiceSupported(t.service) {
		if t.serviceUtils.IsServicePendingFinalization(t.service) {
			t.logger.Info("Service is not supported but pending finalization, running delete flow TODO")
		}
		return nil
	}
	err := t.buildModel(ctx)
	return err
}

func (t *defaultModelBuildTask) buildModel(ctx context.Context) error {
	if t.vlbConfig == nil {
		// build default VLBC
		t.vlbConfig = &v1alpha1.VngcloudLoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.GenerateLBConfigName("svc", t.service.Name),
				Namespace: t.service.Namespace,
			},
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{},
		}
	}

	_, _, subnetId, subnetCIDR, err := t.buildSubnetAndZone(ctx)
	if err != nil {
		return err
	}

	// t.vlbConfig.Labels["TODO"] = "add-labels" // TODO
	t.vlbConfig.Spec.Type = v2.LoadBalancerTypeLayer4
	t.vlbConfig.Spec.SubnetID = subnetId
	t.vlbConfig.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: t.service.APIVersion,
			Kind:       t.service.Kind,
			Name:       t.service.Name,
			UID:        t.service.UID,
			// TODO
		},
	}

	t.vlbConfig.Spec.LoadBalancerID = t.buildLoadBalancerId(ctx)
	t.vlbConfig.Spec.PackageID = t.buildPackageId(ctx)
	t.vlbConfig.Spec.Scheme = t.buildScheme(ctx)
	t.vlbConfig.Spec.EnableAutoscale = t.buildAutoscale(ctx)
	t.vlbConfig.Spec.Tags = t.buildTags(ctx)
	t.vlbConfig.Spec.TargetNodeLabels = t.buildTargetNodeLabels(ctx)
	t.vlbConfig.Spec.IsPoc = t.buildIsPoc(ctx)

	if t.vlbConfig.Spec.LoadBalancerName == "" {
		t.vlbConfig.Spec.LoadBalancerName = t.buildLoadBalancerName(ctx)
	}

	if err := t.buildPoolsAndListeners(ctx); err != nil {
		return err
	}
	if isAutoCreateSecGroup, secgroupIds := t.buildIsAutoCreateSecGroup(ctx); !isAutoCreateSecGroup {
		t.vlbConfig.Spec.AutoManageSecurityGroupRules = nil
		t.vlbConfig.Spec.AttachSecurityGroupsToNodes = secgroupIds
	} else {
		secgroupRules, err := t.buildDefaultSecurityGroupRule(ctx, subnetCIDR)
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

// buildSubnetAndZone tries to get subnet and zone from annotations.
// It will try to get from load-balancer-id annotation.
// If not found, it will try to get from prefer subnet id annotation.
// If not found, it will try to get from prefer zone id annotation.
// If not found, it will use default subnet and zone.
func (t *defaultModelBuildTask) buildSubnetAndZone(ctx context.Context) (zone common.Zone, networkId string, subnetId string, subnetCIDR string, _err error) {
	zone = t.defaultZone
	networkId = t.defaultNetworkId
	subnetId = t.defaultSubnetId
	subnetCIDR = t.defaultSubnetCIDR
	_err = nil

	// try to get from load-balancer-id annotation
	if lbID := t.buildLoadBalancerId(ctx); lbID != nil {
		lb, err := t.vngcloudRepo.GetLoadBalancerByID(ctx, *lbID)
		if err != nil || lb == nil {
			t.logger.Errorf("Failed to get load balancer by id %s: %s.", *lbID, err)
			return common.Zone(""), "", "", "", errors.New("failed to get load balancer by id " + *lbID + ": " + err.Error())
		}
		if lb.SubnetID == t.defaultSubnetId {
			return
		}
		subnet, err := t.vngcloudRepo.GetSubnetByID(ctx, t.defaultNetworkId, lb.SubnetID)
		if err != nil || subnet == nil {
			t.logger.Errorf("Failed to get subnet: %s.", err)
			return common.Zone(""), "", "", "", errors.New("failed to get subnet: " + err.Error())
		}
		return common.Zone(subnet.ZoneID), t.defaultNetworkId, subnet.Id, subnet.Cidr, nil
	}

	// try to get zone, subnet from prefer subnet id annotation
	var preferSubnetId string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferSubnetID, &preferSubnetId, t.service.Annotations)
	if preferSubnetId != "" {
		if preferSubnetId == t.defaultSubnetId {
			return
		}
		subnet, err := t.vngcloudRepo.GetSubnetByID(ctx, t.defaultNetworkId, preferSubnetId)
		if err != nil || subnet == nil {
			t.logger.Errorf("Failed to get prefer subnet: %s.", err)
			return common.Zone(""), "", "", "", errors.New("failed to get prefer subnet: " + err.Error())
		}
		return common.Zone(subnet.ZoneID), t.defaultNetworkId, subnet.Id, subnet.Cidr, nil
	}

	// try to get zone, subnet from prefer zone id annotation
	var preferZoneId string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferZoneID, &preferZoneId, t.service.Annotations)
	if preferZoneId != "" {
		if common.Zone(preferZoneId) == t.defaultZone {
			return
		}

		nodes := &corev1.NodeList{}
		err := t.k8sRepo.ListNode(ctx, nodes)
		if err != nil {
			t.logger.Errorf("Failed to list nodes: %s.", err)
			return common.Zone(""), "", "", "", errors.New("failed to list nodes: " + err.Error())
		}

		providerIds := utils.GetListProviderIdFromNodeList(nodes)
		for _, providerId := range providerIds {
			_zone, _networkID, _subnetID, _subnetCIDR, err := t.vngcloudRepo.GetServerNetworkInfo(ctx, providerId)
			if err != nil {
				continue
			}
			if string(_zone) == preferZoneId {
				return _zone, _networkID, _subnetID, _subnetCIDR, nil
			}
		}
		t.logger.Errorf("Failed to find subnet in prefer zone %s.", preferZoneId)
		return common.Zone(""), "", "", "", errors.New("failed to find subnet in prefer zone " + preferZoneId)
	}

	return t.defaultZone, t.defaultNetworkId, t.defaultSubnetId, t.defaultSubnetCIDR, nil
}

func (t *defaultModelBuildTask) buildLoadBalancerId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &option, t.service.Annotations)
	if option != "" {
		return &option
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

func (t *defaultModelBuildTask) buildAutoscale(_ context.Context) *bool {
	var autoscale bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableAutoscale, &autoscale, t.service.Annotations)
	if err != nil {
		t.logger.Warnf("Failed to parse autoscale annotation: %s", err)
		return nil
	}
	if !isExist {
		return nil
	}
	return &autoscale
}

func (t *defaultModelBuildTask) buildTags(_ context.Context) map[string]string {
	option := make(map[string]string)
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTags, &option, t.service.Annotations)
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTags, nil)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildTargetNodeLabels(_ context.Context) map[string]string {
	option := make(map[string]string)
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTargetNodeLabels, &option, t.service.Annotations)
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string, using default value %v",
			annotations.SuffixTargetNodeLabels, nil)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildIsPoc(_ context.Context) *bool {
	var isPoc bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC, &isPoc, t.service.Annotations)
	if err != nil {
		t.logger.Warnf("Failed to parse isPOC annotation: %s", err)
		return nil
	}
	if !isExist {
		// try deprecated annotation
		isExist, err = t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC2, &isPoc, t.service.Annotations)
		if err != nil {
			t.logger.Warnf("Failed to parse is-poc annotation: %s", err)
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
