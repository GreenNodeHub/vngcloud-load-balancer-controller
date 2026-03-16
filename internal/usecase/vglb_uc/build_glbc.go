package vglb_uc

import (
	"context"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type defaultModelBuildTask struct {
	cfg              *config.Config
	logger           *logrus.Entry
	vglb             *v1alpha1.VngcloudGlobalLoadBalancer
	servicePointer   *corev1.Service
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	annotationParser annotations.Parser
	endpointResolver utils.EndpointResolver

	// Network info
	defaultRegion     string
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
}

func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if err := t.buildGlobalLoadBalancerConfig(ctx); err != nil {
		return err
	}

	// update VGLB status address from GLBC
	address := t.getGLBCAddress(ctx)
	if address != "" {
		err := t.k8sRepo.PatchMutateStatusVngcloudGlobalLoadBalancer(ctx, t.vglb, func(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer) bool {
			if obj.Status.Address == address {
				return false
			}
			obj.Status.Address = address
			return true
		})
		if err != nil {
			t.logger.Errorf("failed to update VGLB status address: %v", err)
			return err
		}
	}
	return nil
}

func (t *defaultModelBuildTask) buildGlobalLoadBalancerConfig(ctx context.Context) error {
	// Get the Service with the same name and namespace as VGLB
	svc, err := t.k8sRepo.GetService(ctx, types.NamespacedName{
		Namespace: t.vglb.Namespace,
		Name:      t.vglb.Name,
	})
	if err != nil {
		t.logger.Warnf("Service %s/%s not found, skipping pools and listeners generation: %v", t.vglb.Namespace, t.vglb.Name, err)
		svc = nil // Continue without service - only basic GLBC fields will be set
	}

	// list GLBC by label selector
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err = t.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList, client.InNamespace(t.vglb.Namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: t.vglb.Name,
		domain.LabelOwnerResourceKind: t.vglb.Kind,
		domain.LabelOwnerResourceUid:  string(t.vglb.UID),
	})
	if err != nil {
		t.logger.Errorf("failed to list GLBC: %v", err)
		return err
	}
	if len(glbcList.Items) > 1 {
		t.logger.Errorf("found multiple GLBC for VGLB %s/%s", t.vglb.Namespace, t.vglb.Name)
		return errors.New("found multiple GLBC for VGLB " + t.vglb.Namespace + "/" + t.vglb.Name)
	}

	glbConfig := &v1alpha1.GlobalLoadBalancerConfig{}
	isCreated := false
	oldGLBConfig := glbConfig.DeepCopy()
	if len(glbcList.Items) == 1 {
		glbConfig = &glbcList.Items[0]
		isCreated = true
		oldGLBConfig = glbConfig.DeepCopy()
	} else {
		glbConfig = &v1alpha1.GlobalLoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    t.vglb.Namespace,
				GenerateName: t.vglb.Name + "-",
			},
			Spec: v1alpha1.GlobalLoadBalancerConfigSpec{},
		}
		isCreated = false
	}

	// Set labels for ownership tracking
	if glbConfig.Labels == nil {
		glbConfig.Labels = make(map[string]string)
	}
	glbConfig.Labels[domain.LabelOwnerResourceName] = t.vglb.Name
	glbConfig.Labels[domain.LabelOwnerResourceKind] = t.vglb.Kind
	glbConfig.Labels[domain.LabelOwnerResourceUid] = string(t.vglb.GetUID())

	// Build spec from VGLB annotations
	glbConfig.Spec.LoadBalancerId = t.buildLoadBalancerId(ctx)
	glbConfig.Spec.Name = t.buildLoadBalancerName(ctx)
	glbConfig.Spec.PackageId = t.buildPackageId(ctx)
	glbConfig.Spec.Description = t.buildDescription(ctx)
	glbConfig.Spec.Type = t.buildType(ctx)

	// Build pools and listeners from Service if available
	if svc != nil {
		t.servicePointer = svc
		pools, listeners, err := t.buildPoolsAndListeners(ctx, nil)
		if err != nil {
			t.logger.Errorf("failed to build pools and listeners: %v", err)
			return err
		}
		glbConfig.Spec.GlobalPools = pools
		glbConfig.Spec.GlobalListeners = listeners
	}

	// Create or update GLBC
	if !isCreated {
		err = t.k8sRepo.CreateGlobalLoadBalancerConfig(ctx, glbConfig)
		if err != nil {
			t.logger.Errorf("failed to create GLBC: %v", err)
			return err
		}
	} else {
		// Only patch if spec actually changed to avoid unnecessary generation increments
		if !glbcSpecEqual(oldGLBConfig.Spec, glbConfig.Spec) {
			err = t.k8sRepo.PatchGlobalLoadBalancerConfig(ctx, glbConfig, client.MergeFrom(oldGLBConfig))
			if err != nil {
				t.logger.Errorf("failed to patch GLBC: %v", err)
				return err
			}
		}
	}

	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &option, t.vglb.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context) string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerName, &option, t.vglb.Annotations)
	if option != "" {
		return option
	}
	// Default name based on VGLB name
	// GLB name only allows a-z, A-Z, 0-9, '_', '.' and must be 5-50 characters
	name := "glb_" + t.vglb.Namespace + "_" + t.vglb.Name
	name = strings.ReplaceAll(name, "-", "_")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

func (t *defaultModelBuildTask) buildPackageId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixPackageID, &option, t.vglb.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildDescription(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixDescription, &option, t.vglb.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildType(_ context.Context) global.GlobalLoadBalancerType {
	// Default to Layer4 for global load balancer
	return global.GlobalLoadBalancerTypeLayer4
}

// getGLBCAddress gets the primary address from GLBC status
func (t *defaultModelBuildTask) getGLBCAddress(ctx context.Context) string {
	// list GLBC by label selector
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := t.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList, client.InNamespace(t.vglb.Namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: t.vglb.Name,
		domain.LabelOwnerResourceKind: t.vglb.Kind,
		domain.LabelOwnerResourceUid:  string(t.vglb.UID),
	})
	if err != nil {
		t.logger.Warnf("failed to list GLBC: %v", err)
		return ""
	}
	if len(glbcList.Items) != 1 {
		return ""
	}

	// Return first domain if available
	if len(glbcList.Items[0].Status.Domains) > 0 {
		return glbcList.Items[0].Status.Domains[0]
	}
	// Otherwise return first VIP address
	if len(glbcList.Items[0].Status.Vips) > 0 {
		return glbcList.Items[0].Status.Vips[0].Address
	}
	return ""
}

// glbcSpecEqual compares two GlobalLoadBalancerConfigSpec for equality.
// Returns true if specs are equal, false otherwise.
func glbcSpecEqual(a, b v1alpha1.GlobalLoadBalancerConfigSpec) bool {
	return reflect.DeepEqual(a, b)
}
