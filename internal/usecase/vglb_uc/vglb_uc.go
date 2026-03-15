package vglb_uc

import (
	"context"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
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
	defaultZone       common.Zone
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
	logger := contexts.NewContext(ctx).Log()

	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		logger.Errorf("failed to list nodes: %v", err)
		return err
	}
	if len(nodes.Items) == 0 {
		return errors.New("no nodes found in cluster")
	}

	// check if network info is available
	if uc.defaultNetworkId == "" || uc.defaultSubnetId == "" || uc.defaultSubnetCIDR == "" || uc.defaultZone == "" {
		// get provider ID from first node
		firstProviderId := utils.GetProviderIdFromNode(&nodes.Items[0])
		if firstProviderId == "" {
			return errors.New("failed to get provider ID from node")
		}
		uc.defaultZone, uc.defaultNetworkId, uc.defaultSubnetId, uc.defaultSubnetCIDR, err = uc.vngcloudRepo.GetServerNetworkInfo(ctx, firstProviderId)
		if err != nil {
			logger.Errorf("failed to get default network info: %v", err)
			return err
		}
		if uc.defaultNetworkId == "" || uc.defaultSubnetId == "" || uc.defaultSubnetCIDR == "" || uc.defaultZone == "" {
			return errors.New("default network info is incomplete")
		}
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

		defaultZone:       uc.defaultZone,
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

	logger := contexts.NewContext(ctx).Log()

	// delete GlobalLoadBalancerConfig created by this VGLB
	stillExist, err := uc.deleteGlobalLoadBalancerConfig(ctx, vglb)
	if err != nil {
		logger.Errorf("failed to delete GlobalLoadBalancerConfig for VGLB %s/%s: %v", vglb.Namespace, vglb.Name, err)
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
		domain.LabelOwnerResourceKind: vglb.Kind,
		domain.LabelOwnerResourceUid:  string(vglb.UID),
	})
	if err != nil {
		logger.Errorf("failed to list GLBCs by label: %v", err)
		return nil, err
	}

	// If no GLBCs found, nothing to delete
	if len(glbcList.Items) == 0 {
		logger.Debug("No GlobalLoadBalancerConfigs found to delete")
		return nil, nil
	}

	// Delete all GLBCs found (non-blocking)
	var stillExist []string
	for _, glbc := range glbcList.Items {
		// If not already being deleted, initiate deletion
		if glbc.DeletionTimestamp.IsZero() {
			err = uc.k8sRepo.DeleteGlobalLoadBalancerConfig(ctx, &glbc)
			if client.IgnoreNotFound(err) != nil {
				logger.Errorf("failed to delete GLBC %s/%s: %v", glbc.Namespace, glbc.Name, err)
				return nil, err
			}
		}

		// Resource still exists (either just deleted or deletion in progress)
		stillExist = append(stillExist, "glbc:"+glbc.Namespace+"/"+glbc.Name)
	}

	if len(stillExist) == 0 {
		logger.Info("All GlobalLoadBalancerConfigs successfully deleted")
	}
	return stillExist, nil
}
