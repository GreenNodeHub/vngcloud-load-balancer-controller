package service_uc

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type defaultModelBuildTask struct {
	clusterId string
	// vpcID            string
	annotationParser annotations.Parser
	serviceUtils     service.ServiceUtils
	cniDetector      utils.CniDetector

	logger           *logrus.Entry
	service          *corev1.Service
	vngcloudRepo     repository.IVngCloudRepository
	k8sRepo          repository.IK8sRepository
	nameHelper       utils.NameHelper
	endpointResolver utils.EndpointResolver

	// this is the default zone, networkId, subnetId, subnetCIDR
	defaultZone       common.Zone
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string

	// this is the current vlb config
	zoneId     common.Zone
	subnetId   string
	subnetCidr string
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if !t.serviceUtils.IsServiceSupported(t.service) {
		if t.serviceUtils.IsServicePendingFinalization(t.service) {
			t.logger.Info("Service is not supported but pending finalization, running delete flow TODO")
		}
		return nil
	}
	if err := t.buildVngcloudLoadBalancerConfig(ctx); err != nil {
		return err
	}
	if err := t.buildNodeSecurityGroup(ctx); err != nil {
		return err
	}

	// update service address
	address := t.getVLBCAddress(ctx)
	if address != "" {
		err := t.k8sRepo.UpdateServiceStatusAddress(ctx, utils.NamespacedName(t.service), address)
		if err != nil {
			t.logger.Errorf("failed to update service status address: %v", err)
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildVngcloudLoadBalancerConfig(ctx context.Context) error {
	// list VLBC by label selector
	vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
	err := t.k8sRepo.ListVLBC(ctx, vlbcList, client.InNamespace(t.service.Namespace), client.MatchingLabels{
		consts.LabelOwnerResourceName: t.service.Name,
		consts.LabelOwnerResourceType: t.service.Kind,
	})
	if err != nil {
		t.logger.Errorf("failed to list VLBC: %v", err)
		return err
	}
	if len(vlbcList.Items) > 1 {
		t.logger.Errorf("found multiple VLBC for service %s/%s", t.service.Namespace, t.service.Name)
		return errors.New("found multiple VLBC for service " + t.service.Namespace + "/" + t.service.Name)
	}
	vlbConfig := &v1alpha1.VngcloudLoadBalancerConfig{}
	isCreated := false
	oldVLBConfig := vlbConfig.DeepCopy()
	if len(vlbcList.Items) == 1 {
		vlbConfig = &vlbcList.Items[0]
		isCreated = true
		oldVLBConfig = vlbConfig.DeepCopy()
	} else {
		vlbConfig = &v1alpha1.VngcloudLoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.GenerateLBConfigName("svc", t.service.Name),
				Namespace: t.service.Namespace,
			},
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{},
		}
		isCreated = false
	}

	// check if have isIgnore annotation
	var isIgnore bool
	t.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, t.service.Annotations)
	if isIgnore {
		t.logger.Info("Service has ignore load balancer config annotation, skip.")
		return nil
	}

	zoneId, _, subnetId, subnetCidr, err := t.buildSubnetAndZone(ctx)
	if err != nil {
		return err
	}
	t.zoneId = zoneId
	t.subnetId = subnetId
	t.subnetCidr = subnetCidr

	if vlbConfig.Labels == nil {
		vlbConfig.Labels = make(map[string]string)
	}
	vlbConfig.Labels[consts.LabelOwnerResourceName] = t.service.Name // TODO
	vlbConfig.Labels[consts.LabelOwnerResourceType] = t.service.Kind
	vlbConfig.Spec.Type = v2.LoadBalancerTypeLayer4
	vlbConfig.Spec.SubnetId = subnetId
	vlbConfig.Spec.ZoneId = zoneId

	// should not set owner reference because sometimes user want to keep VLBC after service is deleted
	// vlbConfig.OwnerReferences = []metav1.OwnerReference{
	// 	{
	// 		APIVersion: t.service.APIVersion,
	// 		Kind:       t.service.Kind,
	// 		Name:       t.service.Name,
	// 		UID:        t.service.UID,
	// 		// TODO
	// 	},
	// }

	if t.clusterId != "" {
		vlbConfig.Spec.ClusterId = &t.clusterId
	}
	vlbConfig.Spec.LoadBalancerId = t.buildLoadBalancerId(ctx)
	vlbConfig.Spec.PackageId = t.buildPackageId(ctx)
	vlbConfig.Spec.Scheme = t.buildScheme(ctx)
	vlbConfig.Spec.EnableAutoscale = t.buildAutoscale(ctx)
	vlbConfig.Spec.Tags = t.buildTags(ctx)
	vlbConfig.Spec.IsPoc = t.buildIsPoc(ctx)
	vlbConfig.Spec.LoadBalancerName = t.buildLoadBalancerName(ctx)

	targetNodeLabels := t.buildTargetNodeLabels(ctx)
	if pools, listeners, err := t.buildPoolsAndListeners(ctx, targetNodeLabels); err != nil {
		return err
	} else {
		vlbConfig.Spec.Pools = pools
		vlbConfig.Spec.Listeners = listeners
	}

	// create or update VLBC
	if !isCreated {
		err = t.k8sRepo.CreateVLBC(ctx, vlbConfig)
		if err != nil {
			t.logger.Errorf("failed to create VLBC: %v", err)
			return err
		}
	} else {
		err = t.k8sRepo.PatchVLBC(ctx, vlbConfig, client.MergeFrom(oldVLBConfig))
		if err != nil {
			t.logger.Errorf("failed to patch VLBC: %v", err)
			return err
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildNodeSecurityGroup(ctx context.Context) error {
	// list NodeSecurityGroup by label selector
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	err := t.k8sRepo.ListNodeSecurityGroup(ctx, nsgList, client.InNamespace(t.service.Namespace), client.MatchingLabels{
		consts.LabelOwnerResourceName: t.service.Name,
		consts.LabelOwnerResourceType: t.service.Kind,
	})
	if err != nil {
		return err
	}
	if len(nsgList.Items) > 1 {
		t.logger.Errorf("found multiple NodeSecurityGroup for service %s/%s", t.service.Namespace, t.service.Name)
		return errors.New("found multiple NodeSecurityGroup for service " + t.service.Namespace + "/" + t.service.Name)
	}
	nsg := &v1alpha1.NodeSecurityGroup{}
	isCreated := false
	oldNSG := nsg.DeepCopy()
	if len(nsgList.Items) == 1 {
		nsg = &nsgList.Items[0]
		isCreated = true
		oldNSG = nsg.DeepCopy()
	} else {
		nsg = &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      t.nameHelper.GetLoadBalancerDefaultName(),
				Namespace: t.service.Namespace,
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{},
		}
		isCreated = false
	}

	if nsg.Labels == nil {
		nsg.Labels = make(map[string]string)
	}
	nsg.Labels[consts.LabelOwnerResourceName] = t.service.Name // TODO
	nsg.Labels[consts.LabelOwnerResourceType] = t.service.Kind

	targetNodeLabels := t.buildTargetNodeLabels(ctx)
	nsg.Spec.SelectNodeLabels = targetNodeLabels

	// TODO: update nsg.Spec based on annotations
	if isAutoCreateSecGroup, secgroupIds := t.buildIsAutoCreateSecGroup(ctx); !isAutoCreateSecGroup {
		nsg.Spec.ManagedSecurityGroup = nil
		nsg.Spec.AttachSecurityGroups = secgroupIds
	} else {
		secgroupRules, err := t.buildDefaultSecurityGroupRule(ctx, t.subnetCidr, targetNodeLabels)
		if err != nil {
			return err
		}
		nsg.Spec.ManagedSecurityGroup = &v1alpha1.ManagedSecurityGroup{
			Rules:       secgroupRules,
			Name:        t.nameHelper.GetLoadBalancerDefaultName(),
			Description: ptr.To("Automatically created using VNGCLOUD LoadBalancer Controller"),
		}
		nsg.Spec.AttachSecurityGroups = nil
	}

	// create or update NodeSecurityGroup
	if !isCreated {
		err = t.k8sRepo.CreateNodeSecurityGroup(ctx, nsg)
		if err != nil {
			t.logger.Errorf("failed to create NodeSecurityGroup: %v", err)
			return err
		}
	} else {
		err = t.k8sRepo.PatchNodeSecurityGroup(ctx, nsg, client.MergeFrom(oldNSG))
		if err != nil {
			t.logger.Errorf("failed to patch NodeSecurityGroup: %v", err)
			return err
		}
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

// get vlbc address from status
func (t *defaultModelBuildTask) getVLBCAddress(ctx context.Context) string {
	// list VLBC by label selector
	vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
	err := t.k8sRepo.ListVLBC(ctx, vlbcList, client.InNamespace(t.service.Namespace), client.MatchingLabels{
		consts.LabelOwnerResourceName: t.service.Name,
		consts.LabelOwnerResourceType: t.service.Kind,
	})
	if err != nil {
		t.logger.Warnf("failed to list VLBC: %v", err)
		return ""
	}
	if len(vlbcList.Items) > 1 {
		t.logger.Warnf("found multiple VLBC for service %s/%s", t.service.Namespace, t.service.Name)
		return ""
	}
	if len(vlbcList.Items) == 0 {
		return ""
	}
	if vlbcList.Items[0].Status.Address != nil {
		return *vlbcList.Items[0].Status.Address
	}
	return ""
}

// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
// func (t *defaultModelBuildTask) build
