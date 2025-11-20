package service_uc

import (
	"context"
	"errors"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
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

func (uc *serviceUseCase) InitServiceUseCase(ctx context.Context) error {
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

func (uc *serviceUseCase) EnsureServiceUseCase(ctx context.Context, req ctrl.Request) error {
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

func (uc *serviceUseCase) DeleteServiceUseCase(ctx context.Context, req ctrl.Request) error {
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

	// delete LoadBalancerConfig and NodeSecurityGroup created by this ingress concurrently
	errCh := make(chan error, 2)

	go func() {
		errCh <- uc.deleteLoadBalancerConfig(ctx, svc)
	}()

	go func() {
		errCh <- uc.deleteNodeSecurityGroup(ctx, svc)
	}()

	// Collect all errors before returning
	var errs []error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			errs = append(errs, err)
		}
	}

	// Combine and return all errors if any occurred
	if len(errs) > 0 {
		for _, err := range errs {
			logger.Errorf("failed to delete resources for service %s/%s: %v", svc.GetNamespace(), svc.GetName(), err)
		}
		return errors.Join(errs...)
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

func (uc *serviceUseCase) deleteLoadBalancerConfig(ctx context.Context, svc *corev1.Service) error {
	logger := contexts.NewContext(ctx).Log()

	// get all LBCs created by this service by using label selector
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err := uc.k8sRepo.ListLoadBalancerConfig(ctx, lbcList, client.InNamespace(svc.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: svc.GetName(),
		domain.LabelOwnerResourceType: svc.Kind,
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

		// wait until LBC is deleted
		err = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, err := uc.k8sRepo.GetLoadBalancerConfig(ctx, client.ObjectKey{
				Namespace: lbc.Namespace,
				Name:      lbc.Name,
			})
			if err != nil {
				if client.IgnoreNotFound(err) != nil {
					return false, err
				}
				return true, nil // NotFound = successfully deleted
			}
			return false, nil
		})

		if err != nil {
			logger.Errorf("timeout waiting for LoadBalancerConfig %s/%s to be deleted: %v", lbc.Namespace, lbc.Name, err)
			return err
		}
	}
	return nil
}

func (uc *serviceUseCase) deleteNodeSecurityGroup(ctx context.Context, svc *corev1.Service) error {
	logger := contexts.NewContext(ctx).Log()
	// get all NSGs created by this service by using label selector
	// and delete them
	secgroupList := &v1alpha1.NodeSecurityGroupList{}
	err := uc.k8sRepo.ListNodeSecurityGroup(ctx, secgroupList, client.InNamespace(svc.GetNamespace()), client.MatchingLabels{
		domain.LabelOwnerResourceName: svc.GetName(),
		domain.LabelOwnerResourceType: svc.Kind,
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

		// wait until NSG is deleted
		err = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			_, err := uc.k8sRepo.GetNodeSecurityGroup(ctx, client.ObjectKey{
				Namespace: secgroup.Namespace,
				Name:      secgroup.Name,
			})
			if err != nil {
				if client.IgnoreNotFound(err) != nil {
					return false, err
				}
				return true, nil // NotFound = successfully deleted
			}
			return false, nil
		})

		if err != nil {
			logger.Errorf("timeout waiting for NodeSecurityGroup %s/%s to be deleted: %v", secgroup.Namespace, secgroup.Name, err)
			return err
		}
	}
	return nil
}
