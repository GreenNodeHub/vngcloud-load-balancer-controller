// Package alb_gateway_uc translates a Gateway-API Gateway under the
// vngcloud-alb GatewayClass into a LoadBalancerConfig CR that the existing
// LBC controller reconciles against the cloud. The Gateway use case is the
// only writer to the LBC's Spec; the LBC controller remains the only writer
// to the LBC's Status (and to the cloud LB itself).
package alb_gateway_uc

import (
	"context"
	"errors"
	"fmt"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	"k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type albGatewayUseCase struct {
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	endpointResolver utils.EndpointResolver

	// k8sClient is a controller-runtime client used for Gateway-API resources
	// (Gateway, HTTPRoute, VKS*Policy) that don't have typed accessors on
	// repository.K8sRepository. Passed in alongside k8sRepo at construction.
	k8sClient client.Client

	clusterId         string
	defaultNetworkId  string
	defaultSubnetId   string
	defaultSubnetCIDR string
	defaultZone       common.Zone
}

// NewALBGatewayUseCase wires the dependencies the Gateway translator needs.
func NewALBGatewayUseCase(
	clusterId string,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	endpointResolver utils.EndpointResolver,
	k8sClient client.Client,
) usecase.ALBGatewayUseCase {
	return &albGatewayUseCase{
		clusterId:        clusterId,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		endpointResolver: endpointResolver,
		k8sClient:        k8sClient,
	}
}

// InitALBGatewayUseCase prepares cluster-scoped state — same shape as the
// other use cases. We discover default network info from a node so that
// VKSGatewayPolicy.LoadBalancerSpec doesn't need to spell it out.
func (uc *albGatewayUseCase) InitALBGatewayUseCase(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()

	// Mirror ingressUseCase.InitIngressUseCase (only the bits the Gateway
	// path actually needs). Errors are non-fatal at startup; a missing
	// default subnet means a Gateway without an explicit VKSGatewayPolicy
	// LoadBalancerSpec.SubnetID will be rejected with Accepted=False.
	if uc.defaultNetworkId != "" && uc.defaultSubnetId != "" && uc.defaultSubnetCIDR != "" && uc.defaultZone != "" {
		return nil
	}

	nodes, err := uc.listNodesForNetworkProbe(ctx)
	if err != nil {
		logger.Warnf("ALB Gateway init: failed to list nodes: %v", err)
		return nil
	}
	if len(nodes) == 0 {
		logger.Warn("ALB Gateway init: no nodes available; default network info will be lazy-populated on first reconcile")
		return nil
	}
	providerID := utils.GetProviderIdFromNode(nodes[0])
	if providerID == "" {
		logger.Warn("ALB Gateway init: first node has no providerID")
		return nil
	}
	zone, networkID, subnetID, cidr, err := uc.vngcloudRepo.GetServerNetworkInfo(ctx, providerID)
	if err != nil {
		logger.Warnf("ALB Gateway init: failed to read default network info: %v", err)
		return nil
	}
	uc.defaultZone, uc.defaultNetworkId, uc.defaultSubnetId, uc.defaultSubnetCIDR = zone, networkID, subnetID, cidr
	return nil
}

// EnsureALBGatewayUseCase is the entry called by the Gateway reconciler.
func (uc *albGatewayUseCase) EnsureALBGatewayUseCase(ctx context.Context, req ctrl.Request) error {
	logger := contexts.NewContext(ctx).Log()

	gw := &gwv1.Gateway{}
	if err := uc.k8sClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, gw); err != nil {
		if apierrors.IsNotFound(err) {
			// The reconciler queued this Gateway and it disappeared in the meantime.
			// Nothing to do — finalizer-driven cleanup runs on the previous tombstone.
			return nil
		}
		return fmt.Errorf("get Gateway %s/%s: %w", req.Namespace, req.Name, err)
	}

	// Phase 1 only handles the ALB GatewayClass.
	gc, err := uc.fetchGatewayClass(ctx, gw)
	if err != nil {
		return err
	}
	if gc == nil || string(gc.Spec.ControllerName) != consts.GatewayClassControllerNameALB {
		// Not ours — leave it alone. Other controllers (or a future NLB
		// reconciler) own different GatewayClasses.
		return nil
	}

	// Deletion path: finalizer protects orderly cleanup.
	if !gw.DeletionTimestamp.IsZero() {
		return uc.handleDeletion(ctx, gw)
	}
	if err := uc.ensureFinalizer(ctx, gw); err != nil {
		return err
	}

	task := &defaultGatewayBuildTask{
		uc:     uc,
		logger: logger.WithField("gateway", req.Namespace+"/"+req.Name),
		gw:     gw,
	}
	return task.run(ctx)
}

// DeleteALBGatewayUseCase is the explicit-delete path; the deletion-finalizer
// path inside Ensure also handles cleanup.
func (uc *albGatewayUseCase) DeleteALBGatewayUseCase(ctx context.Context, req ctrl.Request) error {
	gw := &gwv1.Gateway{}
	if err := uc.k8sClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, gw); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		if meta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("get Gateway %s/%s: %w", req.Namespace, req.Name, err)
	}
	return uc.handleDeletion(ctx, gw)
}

func (uc *albGatewayUseCase) fetchGatewayClass(ctx context.Context, gw *gwv1.Gateway) (*gwv1.GatewayClass, error) {
	if gw.Spec.GatewayClassName == "" {
		return nil, errors.New("Gateway has empty gatewayClassName")
	}
	gc := &gwv1.GatewayClass{}
	if err := uc.k8sClient.Get(ctx, types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}, gc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get GatewayClass %s: %w", gw.Spec.GatewayClassName, err)
	}
	return gc, nil
}
