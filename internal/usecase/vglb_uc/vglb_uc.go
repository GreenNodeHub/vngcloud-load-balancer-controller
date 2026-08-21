package vglb_uc

import (
	"context"
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

type vglbUseCase struct {
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

func NewVngcloudGlobalLoadBalancerUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	annotationParser annotations.Parser,
	endpointResolver utils.EndpointResolver,
) usecase.VngcloudGlobalLoadBalancerUseCase {
	return &vglbUseCase{
		cfg:              cfg,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		endpointResolver: endpointResolver,
	}
}

func (uc *vglbUseCase) InitVngcloudGlobalLoadBalancerUseCase(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		return errors.Wrap(err, "list nodes")
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

func (uc *vglbUseCase) EnsureVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error {
	vglb, err := uc.k8sRepo.GetVngcloudGlobalLoadBalancer(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// Perform the actual reconciliation
	err = uc.ensure(ctx, vglb)

	return err
}

func (uc *vglbUseCase) ensure(ctx context.Context, vglb *v1alpha1.VngcloudGlobalLoadBalancer) error {
	logger := contexts.NewContext(ctx).Log()

	task := &defaultModelBuildTask{
		cfg:              uc.cfg,
		logger:           logger,
		vglb:             vglb,
		k8sRepo:          uc.k8sRepo,
		vngcloudRepo:     uc.vngcloudRepo,
		annotationParser: uc.annotationParser,
		endpointResolver: uc.endpointResolver,
		nameHelper:       utils.NewNameHelper(uc.cfg.Cluster.ClusterID, "vglb", vglb.GetNamespace(), vglb.GetName()),

		defaultRegion:     uc.defaultRegion,
		defaultNetworkId:  uc.defaultNetworkId,
		defaultSubnetId:   uc.defaultSubnetId,
		defaultSubnetCIDR: uc.defaultSubnetCIDR,
	}

	return task.run(ctx)
}

func (uc *vglbUseCase) DeleteVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error {
	vglb, err := uc.k8sRepo.GetVngcloudGlobalLoadBalancer(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	// delete GlobalLoadBalancerConfig created by this VGLB
	stillExist, err := uc.deleteGlobalLoadBalancerConfig(ctx, vglb)
	if err != nil {
		return err
	}

	// If resources still exist, return requeue error
	if len(stillExist) > 0 {
		return errs.NewRequeueNeededAfter(
			"waiting for resources to be deleted: "+strings.Join(stillExist, ", "),
			2*time.Second,
		)
	}

	return nil
}

func (uc *vglbUseCase) deleteGlobalLoadBalancerConfig(ctx context.Context, vglb *v1alpha1.VngcloudGlobalLoadBalancer) ([]string, error) {
	logger := contexts.NewContext(ctx).Log()

	// get all GLBCs created by this VGLB by using label selector
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := uc.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList, client.InNamespace(vglb.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: vglb.GetName(),
		domain.LabelOwnerResourceKind: domain.KindVngcloudGlobalLoadBalancer,
		domain.LabelOwnerResourceUid:  string(vglb.UID),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "list GLBCs owned by VGLB %s/%s", vglb.Namespace, vglb.Name)
	}

	// If no GLBCs found, nothing to delete
	if len(glbcList.Items) == 0 {
		logger.Debug("No GlobalLoadBalancerConfigs found to delete")
		return nil, nil
	}

	// Delete all GLBCs found (non-blocking)
	stillExist := make([]string, 0, len(glbcList.Items))
	for _, glbc := range glbcList.Items {
		// If not already being deleted, initiate deletion
		if glbc.DeletionTimestamp.IsZero() {
			err = uc.k8sRepo.DeleteGlobalLoadBalancerConfig(ctx, &glbc)
			if client.IgnoreNotFound(err) != nil {
				return nil, errors.Wrapf(err, "delete GLBC %s/%s", glbc.Namespace, glbc.Name)
			}
		}

		// Resource still exists (either just deleted or deletion in progress)
		stillExist = append(stillExist, "glbc:"+glbc.Namespace+"/"+glbc.Name)
	}

	return stillExist, nil
}
