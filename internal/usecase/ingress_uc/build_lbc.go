package ingress_uc

import (
	"context"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type defaultModelBuildTask struct {
	clusterId string
	// vpcID            string
	annotationParser annotations.Parser
	ingressUtils     ingress.IngressUtils
	cniDetector      utils.CniDetector

	logger           *logrus.Entry
	ingress          *networkingv1.Ingress
	vngcloudRepo     repository.VngCloudRepository
	k8sRepo          repository.K8sRepository
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
	if !t.ingressUtils.IsIngressSupported(t.ingress) {
		if t.ingressUtils.IsIngressPendingFinalization(t.ingress) {
			return errs.NewRequeueNeeded("ingress is not supported but pending finalization, re-run delete flow")
		}
		return nil
	}
	if err := t.buildLoadBalancerConfig(ctx); err != nil {
		return err
	}
	if err := t.buildNodeSecurityGroup(ctx); err != nil {
		return err
	}

	// update ingress address
	address := t.getLBCAddress(ctx)
	if address != "" {
		err := t.k8sRepo.UpdateIngressStatusAddress(ctx, k8s.NamespacedName(t.ingress), address)
		if err != nil {
			t.logger.Errorf("failed to update ingress status address: %v", err)
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerConfig(ctx context.Context) error {
	// list LBC by label selector
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err := t.k8sRepo.ListLoadBalancerConfig(ctx, lbcList, client.InNamespace(t.ingress.Namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: t.ingress.Name,
		domain.LabelOwnerResourceKind: t.ingress.Kind,
		domain.LabelOwnerResourceUid:  string(t.ingress.UID),
	})
	if err != nil {
		t.logger.Errorf("failed to list LBC: %v", err)
		return err
	}
	if len(lbcList.Items) > 1 {
		t.logger.Errorf("found multiple LBC for ingress %s/%s", t.ingress.Namespace, t.ingress.Name)
		return errors.New("found multiple LBC for ingress " + t.ingress.Namespace + "/" + t.ingress.Name)
	}
	lbConfig := &v1alpha1.LoadBalancerConfig{}
	isCreated := false
	oldLBConfig := lbConfig.DeepCopy()
	if len(lbcList.Items) == 1 {
		lbConfig = &lbcList.Items[0]
		isCreated = true
		oldLBConfig = lbConfig.DeepCopy()
	} else {
		lbConfig = &v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    t.ingress.Namespace,
				GenerateName: t.ingress.Name + "-",
			},
			Spec: v1alpha1.LoadBalancerConfigSpec{},
		}
		isCreated = false
	}

	// check if have isIgnore annotation
	var isIgnore bool
	t.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, t.ingress.Annotations)
	if isIgnore {
		t.logger.Info("Ingress has ignore load balancer config annotation, skip.")
		return nil
	}

	zoneId, networkId, subnetId, subnetCidr, err := t.buildSubnetAndZone(ctx)
	if err != nil {
		return err
	}
	t.zoneId = zoneId
	t.subnetId = subnetId
	t.subnetCidr = subnetCidr

	if lbConfig.Labels == nil {
		lbConfig.Labels = make(map[string]string)
	}
	lbConfig.Labels[domain.LabelOwnerResourceName] = t.ingress.Name
	lbConfig.Labels[domain.LabelOwnerResourceKind] = t.ingress.Kind
	lbConfig.Labels[domain.LabelOwnerResourceUid] = string(t.ingress.UID)
	lbConfig.Spec.Type = v2.LoadBalancerTypeLayer7
	lbConfig.Spec.SubnetId = subnetId
	lbConfig.Spec.VpcId = networkId
	lbConfig.Spec.ZoneId = zoneId

	// should not set owner reference because sometimes user want to keep LBC after ingress is deleted
	// lbConfig.OwnerReferences = []metav1.OwnerReference{...}

	if t.clusterId != "" {
		lbConfig.Spec.ClusterId = &t.clusterId
	}
	lbConfig.Spec.LoadBalancerId = t.buildLoadBalancerId(ctx)
	lbConfig.Spec.PackageId = t.buildPackageId(ctx)
	lbConfig.Spec.Scheme = t.buildScheme(ctx)
	lbConfig.Spec.PrivateSubnetId = t.buildPrivateSubnetId(ctx)
	lbConfig.Spec.EnableAutoscale = t.buildAutoscale(ctx)
	lbConfig.Spec.Tags = t.buildTags(ctx)
	lbConfig.Spec.IsPoc = t.buildIsPoc(ctx)
	lbConfig.Spec.LoadBalancerName = t.buildLoadBalancerName(ctx)

	targetNodeLabels := t.buildTargetNodeLabels(ctx)
	if pools, listeners, err := t.buildPoolsAndListeners(ctx, targetNodeLabels); err != nil {
		return err
	} else {
		lbConfig.Spec.Pools = pools
		lbConfig.Spec.Listeners = listeners
	}

	createCertificates, err := t.buildCreateCertificates(ctx)
	if err != nil {
		return err
	}
	lbConfig.Spec.CreateCertificates = createCertificates

	// create or update LBC
	if !isCreated {
		err = t.k8sRepo.CreateLoadBalancerConfig(ctx, lbConfig)
		if err != nil {
			t.logger.Errorf("failed to create LBC: %v", err)
			return err
		}
	} else {
		// Only patch if spec actually changed to avoid unnecessary generation increments
		if !lbcSpecEqual(oldLBConfig.Spec, lbConfig.Spec) {
			err = t.k8sRepo.PatchLoadBalancerConfig(ctx, lbConfig, client.MergeFrom(oldLBConfig))
			if err != nil {
				t.logger.Errorf("failed to patch LBC: %v", err)
				return err
			}
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildNodeSecurityGroup(ctx context.Context) error {
	// list NodeSecurityGroup by label selector
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	err := t.k8sRepo.ListNodeSecurityGroup(ctx, nsgList, client.InNamespace(t.ingress.Namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: t.ingress.Name,
		domain.LabelOwnerResourceKind: t.ingress.Kind,
		domain.LabelOwnerResourceUid:  string(t.ingress.UID),
	})
	if err != nil {
		return err
	}
	if len(nsgList.Items) > 1 {
		t.logger.Errorf("found multiple NodeSecurityGroup for ingress %s/%s", t.ingress.Namespace, t.ingress.Name)
		return errors.New("found multiple NodeSecurityGroup for ingress " + t.ingress.Namespace + "/" + t.ingress.Name)
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
				Namespace: t.ingress.Namespace,
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{},
		}
		isCreated = false
	}

	if nsg.Labels == nil {
		nsg.Labels = make(map[string]string)
	}
	nsg.Labels[domain.LabelOwnerResourceName] = t.ingress.Name
	nsg.Labels[domain.LabelOwnerResourceKind] = t.ingress.Kind
	nsg.Labels[domain.LabelOwnerResourceUid] = string(t.ingress.UID)

	targetNodeLabels := t.buildTargetNodeLabels(ctx)
	nsg.Spec.SelectNodeLabels = targetNodeLabels

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
		// Only patch if spec actually changed to avoid unnecessary generation increments
		if !nsgSpecEqual(oldNSG.Spec, nsg.Spec) {
			err = t.k8sRepo.PatchNodeSecurityGroup(ctx, nsg, client.MergeFrom(oldNSG))
			if err != nil {
				t.logger.Errorf("failed to patch NodeSecurityGroup: %v", err)
				return err
			}
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildPackageId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPackageID, &option, t.ingress.Annotations)
	if option == "" {
		return nil
	}
	return &option
}

func (t *defaultModelBuildTask) buildScheme(_ context.Context) *v2.LoadBalancerScheme {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixScheme, &option, t.ingress.Annotations)
	switch strings.ToLower(option) {
	case strings.ToLower(string(v2.InternalLoadBalancerScheme)):
		value := v2.InternalLoadBalancerScheme
		return &value
	case strings.ToLower(string(v2.InternetLoadBalancerScheme)):
		value := v2.InternetLoadBalancerScheme
		return &value
	case strings.ToLower(string(v2.InterVpcLoadBalancerScheme)):
		value := v2.InterVpcLoadBalancerScheme
		return &value
	default:
		return nil
	}
}

func (t *defaultModelBuildTask) buildPrivateSubnetId(_ context.Context) *string {
	var option string
	// Check new annotation first (higher priority)
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPrivateSubnetID, &option, t.ingress.Annotations)
	if option != "" {
		return &option
	}

	// Fall back to deprecated annotation
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixBackendSubnetID, &option, t.ingress.Annotations)
	if option != "" {
		t.logger.Warnf("Annotation '%s' is deprecated, please use '%s' instead",
			annotations.SuffixBackendSubnetID, annotations.SuffixPrivateSubnetID)
		return &option
	}

	return nil
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
		if lb.BackendSubnetID == t.defaultSubnetId {
			return
		}
		subnet, err := t.vngcloudRepo.GetSubnetByID(ctx, t.defaultNetworkId, lb.BackendSubnetID)
		if err != nil || subnet == nil {
			t.logger.Errorf("Failed to get subnet: %s.", err)
			return common.Zone(""), "", "", "", errors.New("failed to get subnet: " + err.Error())
		}
		return common.Zone(subnet.ZoneID), t.defaultNetworkId, subnet.Id, subnet.Cidr, nil
	}

	// try to get zone, subnet from prefer subnet id annotation
	var preferSubnetId string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferSubnetID, &preferSubnetId, t.ingress.Annotations)
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
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPreferZoneID, &preferZoneId, t.ingress.Annotations)
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
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &option, t.ingress.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context) string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerName, &option, t.ingress.Annotations)
	if option != "" {
		return option
	}
	return t.nameHelper.GetLoadBalancerDefaultName()
}

func (t *defaultModelBuildTask) buildAutoscale(_ context.Context) *bool {
	var autoscale bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableAutoscale, &autoscale, t.ingress.Annotations)
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
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTags, &option, t.ingress.Annotations)
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string.",
			annotations.SuffixTags)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildTargetNodeLabels(_ context.Context) map[string]string {
	option := make(map[string]string)
	exist, err := t.annotationParser.ParseStringMapAnnotation(annotations.SuffixTargetNodeLabels, &option, t.ingress.Annotations)
	if err != nil {
		t.logger.Warnf("Invalid annotation \"%s\" value, must be a map[string]string.",
			annotations.SuffixTargetNodeLabels)
		return nil
	}
	if !exist {
		return nil
	}
	return option
}

func (t *defaultModelBuildTask) buildIsPoc(_ context.Context) *bool {
	var isPoc bool
	isExist, err := t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC, &isPoc, t.ingress.Annotations)
	if err != nil {
		t.logger.Warnf("Failed to parse isPOC annotation: %s", err)
		return nil
	}
	if !isExist {
		// try deprecated annotation
		isExist, err = t.annotationParser.ParseBoolAnnotation(annotations.SuffixIsPOC2, &isPoc, t.ingress.Annotations)
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

// get lbc address from status
func (t *defaultModelBuildTask) getLBCAddress(ctx context.Context) string {
	// list LBC by label selector
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err := t.k8sRepo.ListLoadBalancerConfig(ctx, lbcList, client.InNamespace(t.ingress.Namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: t.ingress.Name,
		domain.LabelOwnerResourceKind: t.ingress.Kind,
		domain.LabelOwnerResourceUid:  string(t.ingress.UID),
	})
	if err != nil {
		t.logger.Warnf("failed to list LBC: %v", err)
		return ""
	}
	if len(lbcList.Items) > 1 {
		t.logger.Warnf("found multiple LBC for ingress %s/%s", t.ingress.Namespace, t.ingress.Name)
		return ""
	}
	if len(lbcList.Items) == 0 {
		return ""
	}
	if lbcList.Items[0].Status.Address != nil {
		return *lbcList.Items[0].Status.Address
	}
	return ""
}

// lbcSpecEqual compares two LoadBalancerConfigSpec for equality.
// Returns true if specs are equal, false otherwise.
func lbcSpecEqual(a, b v1alpha1.LoadBalancerConfigSpec) bool {
	return reflect.DeepEqual(a, b)
}

// nsgSpecEqual compares two NodeSecurityGroupSpec for equality.
// Returns true if specs are equal, false otherwise.
func nsgSpecEqual(a, b v1alpha1.NodeSecurityGroupSpec) bool {
	return reflect.DeepEqual(a, b)
}
