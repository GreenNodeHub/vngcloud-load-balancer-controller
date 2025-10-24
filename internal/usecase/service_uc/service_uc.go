package service_uc

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/pkg/errors"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type ServiceUseCase struct {
	cfg              *config.Config
	k8sRepo          repository.IK8sRepository
	vngcloudRepo     repository.IVngCloudRepository
	annotationParser annotations.Parser
	serviceUtils     service.ServiceUtils
	cniDetector      utils.CniDetector
	endpointResolver utils.EndpointResolver
	// k8sClient        client.Client

	cniMode utils.CNIType

	clusterId         string
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
	defaultZone       common.Zone
}

func NewServiceUseCase(
	cfg *config.Config,
	k8sRepo repository.IK8sRepository,
	vngcloudRepo repository.IVngCloudRepository,
	annotationParser annotations.Parser,
	serviceUtils service.ServiceUtils,
	cniDetector utils.CniDetector,
	endpointResolver utils.EndpointResolver,
	// k8sClient client.Client,
) usecase.IServiceUseCase {
	return &ServiceUseCase{
		cfg:              cfg,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		serviceUtils:     serviceUtils,
		cniDetector:      cniDetector,
		endpointResolver: endpointResolver,
		// k8sClient:        k8sClient,
	}
}

func (uc *ServiceUseCase) Init(ctx context.Context) error {
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

	uc.clusterId = uc.cfg.Cluster.ClusterID
	// if clusterID is empty, get from node label
	if uc.cfg.Cluster.ClusterID == "" {
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
	uc.cniMode, err = uc.cniDetector.DetectCNIType(ctx)
	if err != nil {
		logger.Errorf("failed to detect CNI type: %v", err)
		return err
	}
	logger.Infof("Detected CNI type: %s", uc.cniMode)
	return nil
}

func (uc *ServiceUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
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

func (uc *ServiceUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	return nil
}

////////////////////////////////////////////////////////////////

func (uc *ServiceUseCase) ensure(ctx context.Context, req ctrl.Request) error {
	svc, err := uc.k8sRepo.GetService(ctx, req.NamespacedName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	vlbConfigName := utils.GenerateLBConfigName("svc", svc.GetName())
	vlbc, err := uc.k8sRepo.GetVLBC(ctx, types.NamespacedName{Name: vlbConfigName, Namespace: svc.GetNamespace()})
	if client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get VLBC")
		return err
	}
	isCreated := true
	oldVLBConfig := vlbc.DeepCopy()
	if err != nil && client.IgnoreNotFound(err) == nil {
		isCreated = false
		vlbc = nil
	} else {
		// check if have isIgnore annotation
		var isIgnore bool
		uc.annotationParser.ParseBoolAnnotation(annotations.SuffixIgnore, &isIgnore, svc.Annotations)
		if isIgnore {
			logger.Info("Service has ignore load balancer config annotation, skip.")
			return nil
		}

		// save old vlbc for patching later
		oldVLBConfig = vlbc.DeepCopy()
	}

	task := &defaultModelBuildTask{
		annotationParser: uc.annotationParser,
		serviceUtils:     uc.serviceUtils,

		logger:           logger,
		service:          svc,
		vlbConfig:        vlbc,
		vngcloudRepo:     uc.vngcloudRepo,
		k8sRepo:          uc.k8sRepo,
		nameHelper:       utils.NewNameHelper(uc.clusterId, "service", svc.GetNamespace(), svc.GetName()),
		cniMode:          uc.cniMode,
		endpointResolver: uc.endpointResolver,

		defaultZone:       uc.defaultZone,
		defaultNetworkId:  uc.defaultNetworkId,
		defaultSubnetId:   uc.defaultSubnetId,
		defaultSubnetCIDR: uc.defaultSubnetCIDR,
	}

	if err := task.run(ctx); err != nil {
		return err
	}

	if !isCreated {
		err = uc.k8sRepo.CreateVLBC(ctx, task.vlbConfig)
		if err != nil {
			logger.Errorf("failed to create VLBC: %v", err)
			return err
		}
	} else {
		err = uc.k8sRepo.PatchVLBC(ctx, task.vlbConfig, client.MergeFrom(oldVLBConfig))
		if err != nil {
			logger.Errorf("failed to patch VLBC: %v", err)
			return err
		}
	}

	return nil
}
