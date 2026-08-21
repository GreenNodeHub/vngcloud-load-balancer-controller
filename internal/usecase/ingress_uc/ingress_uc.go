package ingress_uc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type ingressUseCase struct {
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	annotationParser annotations.Parser
	ingressUtils     ingress.IngressUtils
	cniDetector      utils.CniDetector
	endpointResolver utils.EndpointResolver

	clusterId         string
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
	defaultZone       common.Zone
}

func NewIngressUseCase(
	clusterId string,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	annotationParser annotations.Parser,
	ingressUtils ingress.IngressUtils,
	cniDetector utils.CniDetector,
	endpointResolver utils.EndpointResolver,
) usecase.IngressUseCase {
	return &ingressUseCase{
		clusterId:        clusterId,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		ingressUtils:     ingressUtils,
		cniDetector:      cniDetector,
		endpointResolver: endpointResolver,
	}
}

func (uc *ingressUseCase) InitIngressUseCase(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()

	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return errors.New("no nodes found in cluster")
	}

	// check if network info is available
	if uc.defaultNetworkId == "" || uc.defaultSubnetId == "" || uc.defaultSubnetCIDR == "" || uc.defaultZone == "" {
		// get provider ID from first node
		// Deterministically, not whichever node the cache handed back first - see FirstNodeByName.
		firstProviderId := utils.GetProviderIdFromNode(utils.FirstNodeByName(nodes))
		if firstProviderId == "" {
			return errors.New("failed to get provider ID from node")
		}
		uc.defaultZone, uc.defaultNetworkId, uc.defaultSubnetId, uc.defaultSubnetCIDR, err = uc.vngcloudRepo.GetServerNetworkInfo(ctx, firstProviderId)
		if err != nil {
			return fmt.Errorf("failed to get default network info from node %s: %w", firstProviderId, err)
		}
		if uc.defaultNetworkId == "" || uc.defaultSubnetId == "" || uc.defaultSubnetCIDR == "" || uc.defaultZone == "" {
			return errors.New("default network info is incomplete")
		}
	}

	// if clusterID is empty, get from node label
	if uc.clusterId == "" {
		clusterID := ""
		for _, node := range nodes.Items {
			if node.Labels != nil && node.Labels["vks.vngcloud.vn/cluster-id"] != "" {
				clusterID = node.Labels["vks.vngcloud.vn/cluster-id"]
				break
			}
		}
		if clusterID == "" {
			return errors.New("no clusterID found, should exist in node label or specify in config")
		}
		uc.clusterId = clusterID
		logger.Infof("ClusterID is empty, get from node label: %s", uc.clusterId)
	}

	// init cni mode
	cniMode, err := uc.cniDetector.DetectCNIType(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect CNI type: %w", err)
	}
	logger.Infof("Detected CNI type: %s", cniMode)
	return nil
}

func (uc *ingressUseCase) EnsureIngressUseCase(ctx context.Context, req ctrl.Request) error {
	err := uc.ensure(ctx, req)
	// some errors should not requeue
	if err != nil {
		switch {
		case domain.IsExceededSecurityGroupPerServerQuota(err),
			domain.IsLoadBalancerNotFound(err):
			err = errs.NewNoNeedRequeue(err.Error())
		}
	}
	return err
}

func (uc *ingressUseCase) DeleteIngressUseCase(ctx context.Context, req ctrl.Request) error {
	ing, err := uc.k8sRepo.GetIngress(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()

	// check if have isIgnore annotation
	var isIgnore bool
	_, _ = uc.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, ing.Annotations)
	if isIgnore {
		logger.Debug("Ingress has ignore load balancer config annotation, skip.")
		return nil
	}

	// delete LoadBalancerConfig and NodeSecurityGroup created by this ingress concurrently
	type deleteResult struct {
		stillExist []string
		err        error
	}
	resultCh := make(chan deleteResult, 2)

	go func() {
		stillExist, err := uc.deleteLoadBalancerConfig(ctx, ing)
		resultCh <- deleteResult{stillExist: stillExist, err: err}
	}()

	go func() {
		stillExist, err := uc.deleteNodeSecurityGroup(ctx, ing)
		resultCh <- deleteResult{stillExist: stillExist, err: err}
	}()

	// Collect results
	var allStillExist []string
	var realErrs []error
	for i := 0; i < 2; i++ {
		result := <-resultCh
		if result.err != nil {
			realErrs = append(realErrs, result.err)
		}
		allStillExist = append(allStillExist, result.stillExist...)
	}

	// If there are real errors, return them immediately
	if len(realErrs) > 0 {
		return errors.Join(realErrs...)
	}

	// If resources still exist, return requeue error
	if len(allStillExist) > 0 {
		return errs.NewRequeueNeededAfter(
			"waiting for resources to be deleted: "+strings.Join(allStillExist, ", "),
			2*time.Second,
		)
	}

	return nil
}

// //////////////////////////////////////////////////////////////

func (uc *ingressUseCase) ensure(ctx context.Context, req ctrl.Request) error {
	ing, err := uc.k8sRepo.GetIngress(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()

	task := &defaultModelBuildTask{
		clusterId:        uc.clusterId,
		annotationParser: uc.annotationParser,
		ingressUtils:     uc.ingressUtils,
		cniDetector:      uc.cniDetector,

		logger:           logger,
		ingress:          ing,
		vngcloudRepo:     uc.vngcloudRepo,
		k8sRepo:          uc.k8sRepo,
		nameHelper:       utils.NewNameHelper(uc.clusterId, "ingress", ing.GetNamespace(), ing.GetName()),
		endpointResolver: uc.endpointResolver,

		defaultZone:       uc.defaultZone,
		defaultNetworkId:  uc.defaultNetworkId,
		defaultSubnetId:   uc.defaultSubnetId,
		defaultSubnetCIDR: uc.defaultSubnetCIDR,
	}

	return task.run(ctx)
}

func (uc *ingressUseCase) deleteLoadBalancerConfig(ctx context.Context, ing *networkingv1.Ingress) ([]string, error) {
	logger := contexts.NewContext(ctx).Log()

	// get all LBCs created by this ingress by using label selector
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err := uc.k8sRepo.ListLoadBalancerConfig(ctx, lbcList, client.InNamespace(ing.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: ing.GetName(),
		domain.LabelOwnerResourceKind: ing.Kind,
		domain.LabelOwnerResourceUid:  string(ing.UID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list LoadBalancerConfigs by label: %w", err)
	}

	// If no LBCs found, nothing to delete
	if len(lbcList.Items) == 0 {
		logger.Debug("No LoadBalancerConfigs found to delete")
		return nil, nil
	}

	// Delete all LBCs found (non-blocking)
	stillExist := make([]string, 0, len(lbcList.Items))
	for _, lbc := range lbcList.Items {
		// If not already being deleted, initiate deletion
		if lbc.DeletionTimestamp.IsZero() {
			err = uc.k8sRepo.DeleteLoadBalancerConfig(ctx, &lbc)
			if client.IgnoreNotFound(err) != nil {
				return nil, fmt.Errorf("failed to delete LoadBalancerConfig %s/%s: %w", lbc.Namespace, lbc.Name, err)
			}
		}

		// Resource still exists (either just deleted or deletion in progress)
		stillExist = append(stillExist, "lbc:"+lbc.Namespace+"/"+lbc.Name)
	}

	return stillExist, nil
}

func (uc *ingressUseCase) deleteNodeSecurityGroup(ctx context.Context, ing *networkingv1.Ingress) ([]string, error) {
	logger := contexts.NewContext(ctx).Log()
	// get all NSGs created by this ingress by using label selector
	secgroupList := &v1alpha1.NodeSecurityGroupList{}
	err := uc.k8sRepo.ListNodeSecurityGroup(ctx, secgroupList, client.InNamespace(ing.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: ing.GetName(),
		domain.LabelOwnerResourceKind: ing.Kind,
		domain.LabelOwnerResourceUid:  string(ing.UID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list NodeSecurityGroups by label: %w", err)
	}

	// If no NSGs found, nothing to delete
	if len(secgroupList.Items) == 0 {
		logger.Debug("No NodeSecurityGroups found to delete")
		return nil, nil
	}

	// Delete all NSGs found (non-blocking)
	stillExist := make([]string, 0, len(secgroupList.Items))
	for _, secgroup := range secgroupList.Items {
		// If not already being deleted, initiate deletion
		if secgroup.DeletionTimestamp.IsZero() {
			err = uc.k8sRepo.DeleteNodeSecurityGroup(ctx, &secgroup)
			if client.IgnoreNotFound(err) != nil {
				return nil, fmt.Errorf("failed to delete NodeSecurityGroup %s/%s: %w", secgroup.Namespace, secgroup.Name, err)
			}
		}

		// Resource still exists (either just deleted or deletion in progress)
		stillExist = append(stillExist, "nsg:"+secgroup.Namespace+"/"+secgroup.Name)
	}

	return stillExist, nil
}
