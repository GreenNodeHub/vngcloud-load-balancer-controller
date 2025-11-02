package service_uc

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type serviceUseCase struct {
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	annotationParser annotations.Parser
	serviceUtils     service.ServiceUtils
	cniDetector      utils.CniDetector
	endpointResolver utils.EndpointResolver

	clusterId         string
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
	defaultZone       common.Zone
}

func NewServiceUseCase(
	clusterId string,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	annotationParser annotations.Parser,
	serviceUtils service.ServiceUtils,
	cniDetector utils.CniDetector,
	endpointResolver utils.EndpointResolver,
) usecase.ServiceUseCase {
	return &serviceUseCase{
		clusterId:        clusterId,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		serviceUtils:     serviceUtils,
		cniDetector:      cniDetector,
		endpointResolver: endpointResolver,
	}
}

func (uc *serviceUseCase) Init(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()

	nodes := &corev1.NodeList{}
	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		logger.Errorf("failed to list nodes: %v", err)
		return err
	}

	// check if network info is available
	if uc.defaultNetworkId == "" || uc.defaultSubnetId == "" || uc.defaultSubnetCIDR == "" || uc.defaultZone == "" {
		providerIds := utils.GetListProviderIdFromNodeList(nodes) // TODO do we need get all provider ids?
		uc.defaultZone, uc.defaultNetworkId, uc.defaultSubnetId, uc.defaultSubnetCIDR, err = uc.vngcloudRepo.GetServerNetworkInfo(ctx, providerIds[0])
		if err != nil {
			logger.Errorf("failed to get default network info: %v", err)
			return err
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
		logger.Errorf("failed to detect CNI type: %v", err)
		return err
	}
	logger.Infof("Detected CNI type: %s", cniMode)
	return nil
}

func (uc *serviceUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
	err := uc.ensure(ctx, req)
	// some errors should not requeue
	if err != nil {
		switch {
		case errs.IsExceededSecurityGroupPerServerQuota(err),
			errs.IsLoadBalancerNotFound(err):
			err = errs.NewNoNeedRequeue(err.Error())
		}
	}
	return err
}

func (uc *serviceUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	svc, err := uc.k8sRepo.GetService(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()

	// check if have isIgnore annotation
	var isIgnore bool
	uc.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, svc.Annotations)
	if isIgnore {
		logger.Info("Service has ignore load balancer config annotation, skip.")
		return nil
	}

	// get all LBCs created by this service by using label selector
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err = uc.k8sRepo.ListLoadBalancerConfig(ctx, lbcList, client.InNamespace(svc.GetNamespace()), client.MatchingLabels{
		consts.LabelOwnerResourceName: svc.GetName(),
		consts.LabelOwnerResourceType: svc.Kind,
	})
	if err != nil {
		logger.Errorf("failed to list LBCs by label: %v", err)
		return err
	}

	// delete all LBCs found
	for _, lbc := range lbcList.Items {
		err = uc.k8sRepo.DeleteLoadBalancerConfig(ctx, &lbc)
		if client.IgnoreNotFound(err) != nil {
			logger.Errorf("failed to delete LBC %s/%s: %v", lbc.Namespace, lbc.Name, err)
			return err
		}
	}

	// get all NSGs created by this service by using label selector
	// and delete them
	secgroupList := &v1alpha1.NodeSecurityGroupList{}
	err = uc.k8sRepo.ListNodeSecurityGroup(ctx, secgroupList, client.InNamespace(svc.GetNamespace()), client.MatchingLabels{
		consts.LabelOwnerResourceName: svc.GetName(),
		consts.LabelOwnerResourceType: svc.Kind,
	})
	if err != nil {
		logger.Errorf("failed to list NodeSecurityGroups by label: %v", err)
		return err
	}

	// delete all NSGs found
	for _, secgroup := range secgroupList.Items {
		err = uc.k8sRepo.DeleteNodeSecurityGroup(ctx, &secgroup)
		if client.IgnoreNotFound(err) != nil {
			logger.Errorf("failed to delete NodeSecurityGroup %s/%s: %v", secgroup.Namespace, secgroup.Name, err)
			return err
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////

func (uc *serviceUseCase) ensure(ctx context.Context, req ctrl.Request) error {
	svc, err := uc.k8sRepo.GetService(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()

	task := &defaultModelBuildTask{
		clusterId:        uc.clusterId,
		annotationParser: uc.annotationParser,
		serviceUtils:     uc.serviceUtils,
		cniDetector:      uc.cniDetector,

		logger:           logger,
		service:          svc,
		vngcloudRepo:     uc.vngcloudRepo,
		k8sRepo:          uc.k8sRepo,
		nameHelper:       utils.NewNameHelper(uc.clusterId, "service", svc.GetNamespace(), svc.GetName()),
		endpointResolver: uc.endpointResolver,

		defaultZone:       uc.defaultZone,
		defaultNetworkId:  uc.defaultNetworkId,
		defaultSubnetId:   uc.defaultSubnetId,
		defaultSubnetCIDR: uc.defaultSubnetCIDR,
	}

	return task.run(ctx)
}
