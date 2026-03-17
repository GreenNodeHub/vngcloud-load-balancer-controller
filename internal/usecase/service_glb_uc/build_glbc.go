package service_glb_uc

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
	service          *corev1.Service
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

// run executes the full reconciliation: build GLBC then update Service status address.
func (t *defaultModelBuildTask) run(ctx context.Context) error {
	if err := t.buildGlobalLoadBalancerConfig(ctx); err != nil {
		return err
	}

	// Update Service status address from GLBC status domains
	address := t.getGLBCAddress(ctx)
	if address != "" {
		err := t.k8sRepo.UpdateServiceStatusAddress(ctx,
			types.NamespacedName{Namespace: t.service.Namespace, Name: t.service.Name},
			address,
		)
		if err != nil {
			t.logger.Errorf("failed to update Service status address: %v", err)
			return err
		}
	}
	return nil
}

// buildGlobalLoadBalancerConfig creates or updates the GLBC owned by this Service.
func (t *defaultModelBuildTask) buildGlobalLoadBalancerConfig(ctx context.Context) error {
	// List GLBC by owner labels (Kind=Service)
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := t.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList,
		client.InNamespace(t.service.Namespace),
		client.MatchingLabels{
			domain.LabelOwnerResourceName: t.service.Name,
			domain.LabelOwnerResourceKind: domain.KindService,
			domain.LabelOwnerResourceUid:  string(t.service.UID),
		},
	)
	if err != nil {
		t.logger.Errorf("failed to list GLBC: %v", err)
		return err
	}
	if len(glbcList.Items) > 1 {
		t.logger.Errorf("found multiple GLBC for Service %s/%s", t.service.Namespace, t.service.Name)
		return errors.New("found multiple GLBC for Service " + t.service.Namespace + "/" + t.service.Name)
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
				Namespace:    t.service.Namespace,
				GenerateName: t.service.Name + "-",
			},
			Spec: v1alpha1.GlobalLoadBalancerConfigSpec{},
		}
		isCreated = false
	}

	// Set labels for ownership tracking
	// Note: K8s strips TypeMeta from objects returned by Get; use KindService constant, not svc.Kind
	if glbConfig.Labels == nil {
		glbConfig.Labels = make(map[string]string)
	}
	glbConfig.Labels[domain.LabelOwnerResourceName] = t.service.Name
	glbConfig.Labels[domain.LabelOwnerResourceKind] = domain.KindService
	glbConfig.Labels[domain.LabelOwnerResourceUid] = string(t.service.GetUID())

	// Build spec from Service annotations (glb.vks.vngcloud.vn/* prefix)
	glbConfig.Spec.LoadBalancerId = t.buildLoadBalancerId(ctx)
	glbConfig.Spec.Name = t.buildLoadBalancerName(ctx)
	glbConfig.Spec.PackageId = t.buildPackageId(ctx)
	glbConfig.Spec.Description = t.buildDescription(ctx)
	glbConfig.Spec.Type = t.buildType(ctx)

	// Build pools and listeners from Service ports
	pools, listeners, err := t.buildPoolsAndListeners(ctx, nil)
	if err != nil {
		t.logger.Errorf("failed to build pools and listeners: %v", err)
		return err
	}
	glbConfig.Spec.GlobalPools = pools
	glbConfig.Spec.GlobalListeners = listeners

	// Create or patch GLBC
	if !isCreated {
		err = t.k8sRepo.CreateGlobalLoadBalancerConfig(ctx, glbConfig)
		if err != nil {
			t.logger.Errorf("failed to create GLBC: %v", err)
			return err
		}
	} else {
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
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBLoadBalancerID, &option, t.service.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildLoadBalancerName(_ context.Context) string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBLoadBalancerName, &option, t.service.Annotations)
	if option != "" {
		return option
	}
	// Default name based on Service namespace and name
	// GLB name only allows a-z, A-Z, 0-9, '_', '.' and must be 5-50 characters
	name := "vks_" + t.service.Namespace + "_" + t.service.Name
	name = strings.ReplaceAll(name, "-", "_")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

func (t *defaultModelBuildTask) buildPackageId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBPackageID, &option, t.service.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildDescription(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixGLBDescription, &option, t.service.Annotations)
	if option != "" {
		return &option
	}
	return nil
}

func (t *defaultModelBuildTask) buildType(_ context.Context) global.GlobalLoadBalancerType {
	// Default to Layer4 for global load balancer
	return global.GlobalLoadBalancerTypeLayer4
}

// getGLBCAddress returns the first domain from the GLBC status, or empty string.
func (t *defaultModelBuildTask) getGLBCAddress(ctx context.Context) string {
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := t.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList,
		client.InNamespace(t.service.Namespace),
		client.MatchingLabels{
			domain.LabelOwnerResourceName: t.service.Name,
			domain.LabelOwnerResourceKind: domain.KindService,
			domain.LabelOwnerResourceUid:  string(t.service.UID),
		},
	)
	if err != nil {
		t.logger.Warnf("failed to list GLBC: %v", err)
		return ""
	}
	if len(glbcList.Items) != 1 {
		return ""
	}

	if len(glbcList.Items[0].Status.Domains) > 0 {
		return glbcList.Items[0].Status.Domains[0]
	}
	return ""
}

// glbcSpecEqual compares two GlobalLoadBalancerConfigSpec for equality.
func glbcSpecEqual(a, b v1alpha1.GlobalLoadBalancerConfigSpec) bool {
	return reflect.DeepEqual(a, b)
}
