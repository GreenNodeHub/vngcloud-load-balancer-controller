package service_glb_uc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

const (
	labelMgmtZone  = "vks.vngcloud.vn/mgmt-zone"
	labelNetworkId = "vks.vngcloud.vn/network-id"
	labelSubnetId  = "vks.vngcloud.vn/subnet-id"
)

var zoneRe = regexp.MustCompile(`\d+[a-z]*$`)

// stripZoneSuffix converts zone to region: "hcm03b" -> "hcm", "sgn01a" -> "sgn", "han01" -> "han"
func stripZoneSuffix(zone string) string {
	return zoneRe.ReplaceAllString(zone, "")
}

type serviceGLBUseCase struct {
	cfg              *config.Config
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	annotationParser annotations.Parser
	endpointResolver utils.EndpointResolver

	// Network info from cluster
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
	defaultRegion     string
}

// NewServiceGLBUseCase creates a new ServiceGLBUseCase.
func NewServiceGLBUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	annotationParser annotations.Parser,
	endpointResolver utils.EndpointResolver,
) usecase.ServiceGLBUseCase {
	return &serviceGLBUseCase{
		cfg:              cfg,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		endpointResolver: endpointResolver,
	}
}

// InitServiceGLBUseCase reads network info from node labels (same pattern as vglb_uc).
func (uc *serviceGLBUseCase) InitServiceGLBUseCase(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return errors.New("no nodes found in cluster")
	}

	// Read network info from node labels
	// Deterministically, not whichever node the cache handed back first - see FirstNodeByName.
	firstNode := utils.FirstNodeByName(nodes)
	rawZone := firstNode.Labels[labelMgmtZone]
	uc.defaultNetworkId = firstNode.Labels[labelNetworkId]
	uc.defaultSubnetId = firstNode.Labels[labelSubnetId]
	uc.defaultRegion = stripZoneSuffix(rawZone)
	uc.defaultSubnetCIDR = ""

	if uc.defaultRegion == "" || uc.defaultNetworkId == "" || uc.defaultSubnetId == "" {
		return errors.Errorf(
			"incomplete network info from node labels: zone=%q (region=%q), networkId=%q, subnetId=%q",
			rawZone, uc.defaultRegion, uc.defaultNetworkId, uc.defaultSubnetId,
		)
	}

	return nil
}

// EnsureServiceGLBUseCase reconciles a Service that has the GLB enable annotation.
func (uc *serviceGLBUseCase) EnsureServiceGLBUseCase(ctx context.Context, req ctrl.Request) error {
	svc, err := uc.k8sRepo.GetService(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()

	task := &defaultModelBuildTask{
		cfg:              uc.cfg,
		logger:           logger,
		service:          svc,
		k8sRepo:          uc.k8sRepo,
		vngcloudRepo:     uc.vngcloudRepo,
		annotationParser: uc.annotationParser,
		endpointResolver: uc.endpointResolver,
		nameHelper:       utils.NewNameHelper(uc.cfg.Cluster.ClusterID, "service-glb", svc.GetNamespace(), svc.GetName()),

		defaultRegion:     uc.defaultRegion,
		defaultNetworkId:  uc.defaultNetworkId,
		defaultSubnetId:   uc.defaultSubnetId,
		defaultSubnetCIDR: uc.defaultSubnetCIDR,
	}

	return task.run(ctx)
}

// DeleteServiceGLBUseCase deletes all GLBCs owned by the Service.
func (uc *serviceGLBUseCase) DeleteServiceGLBUseCase(ctx context.Context, req ctrl.Request) error {
	svc, err := uc.k8sRepo.GetService(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	stillExist, err := uc.deleteGlobalLoadBalancerConfig(ctx, svc)
	if err != nil {
		return err
	}

	if len(stillExist) > 0 {
		return errs.NewRequeueNeededAfter(
			"waiting for resources to be deleted: "+strings.Join(stillExist, ", "),
			2*time.Second,
		)
	}

	return nil
}

// deleteGlobalLoadBalancerConfig deletes all GLBCs with owner labels matching the Service.
func (uc *serviceGLBUseCase) deleteGlobalLoadBalancerConfig(ctx context.Context, svc *corev1.Service) ([]string, error) {
	logger := contexts.NewContext(ctx).Log()

	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := uc.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList, client.InNamespace(svc.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: svc.GetName(),
		domain.LabelOwnerResourceKind: domain.KindService,
		domain.LabelOwnerResourceUid:  string(svc.UID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list GLBCs by label: %w", err)
	}

	if len(glbcList.Items) == 0 {
		logger.Debug("No GlobalLoadBalancerConfigs found to delete")
		return nil, nil
	}

	stillExist := make([]string, 0, len(glbcList.Items))
	for _, glbc := range glbcList.Items {
		if glbc.DeletionTimestamp.IsZero() {
			err = uc.k8sRepo.DeleteGlobalLoadBalancerConfig(ctx, &glbc)
			if client.IgnoreNotFound(err) != nil {
				return nil, fmt.Errorf("failed to delete GLBC %s/%s: %w", glbc.Namespace, glbc.Name, err)
			}
		}
		stillExist = append(stillExist, "glbc:"+glbc.Namespace+"/"+glbc.Name)
	}

	return stillExist, nil
}
