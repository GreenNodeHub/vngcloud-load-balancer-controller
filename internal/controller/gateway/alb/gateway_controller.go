// Package alb hosts the reconciler for Gateway-API Gateway objects under the
// vngcloud-alb GatewayClass. The reconciler is intentionally thin: it owns
// init gating, queue plumbing, and event filtering. All Gateway → LBC
// translation lives in internal/usecase/gateway_uc/alb_gateway_uc.
package alb

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	gwv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	gwhandlers "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)

const albControllerName = "gateway-alb"

// GatewayReconciler reconciles Gateway objects under the vngcloud-alb class.
type GatewayReconciler struct {
	k8sClient client.Client
	scheme    *runtime.Scheme
	useCase   usecase.ALBGatewayUseCase

	initDone                atomic.Bool
	maxConcurrentReconciles int
}

// NewGatewayReconciler wires the dependencies. Called from cmd/main.go.
func NewGatewayReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	useCase usecase.ALBGatewayUseCase,
	maxConcurrentReconciles int,
) *GatewayReconciler {
	return &GatewayReconciler{
		k8sClient: k8sClient,
		scheme:    scheme,
		useCase:   useCase,
		maxConcurrentReconciles: func() int {
			if maxConcurrentReconciles > 0 {
				return maxConcurrentReconciles
			}
			return domain.DefaultMaxConcurrentReconciles
		}(),
	}
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.vks.vngcloud.vn,resources=vksgatewaypolicies;vksbackendpolicies;vkshealthcheckpolicies;vksroutepolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=loadbalancerconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the controller-runtime entry point.
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("ALB Gateway init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	ctx = contexts.NewContext(ctx).
		SetLogName("gw/" + req.Namespace + "/" + req.Name).
		GetContext()
	logger := contexts.NewContext(ctx).Log()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.useCase.EnsureALBGatewayUseCase(ctx, req), logger)
}

// SetupWithManager registers the reconciler with the manager and wires watches:
//   - Gateway: primary, with a class-name predicate so only ALB Gateways enter
//     this controller's queue.
//   - HTTPRoute: secondary, enqueue parent Gateway via parentRefs.
//   - VKSGatewayPolicy: secondary, enqueue every Gateway named in targetRefs.
//   - LoadBalancerConfig: secondary, enqueue the owning Gateway when the LBC's
//     Status changes (Phase F status-mirroring hook).
//   - Service: secondary, enqueue affected parent Gateways via reverse index.
//
// Init is asynchronous so the manager can come up even when vngcloud is
// unreachable; reconciles requeue until init succeeds.
func (r *GatewayReconciler) SetupWithManager(ctx context.Context, mgr manager.Manager) error {
	go func() {
		for {
			if err := r.useCase.InitALBGatewayUseCase(ctx); err != nil {
				ctrl.Log.Error(err, "ALB Gateway init failed; retrying")
				time.Sleep(10 * time.Second)
				continue
			}
			r.initDone.Store(true)
			return
		}
	}()

	return ctrl.NewControllerManagedBy(mgr).
		Named(albControllerName).
		For(&gwv1.Gateway{}, builder.WithPredicates(albClassPredicate(r.k8sClient))).
		Watches(&gwv1.HTTPRoute{}, gwhandlers.RouteToGateway()).
		Watches(&gwv1alpha1.VKSGatewayPolicy{}, gwhandlers.VKSGatewayPolicyToGateway()).
		Watches(&gwv1alpha1.VKSBackendPolicy{}, gwhandlers.VKSBackendPolicyToGateway(r.k8sClient)).
		Watches(&gwv1alpha1.VKSHealthCheckPolicy{}, gwhandlers.VKSHealthCheckPolicyToGateway(r.k8sClient)).
		Watches(&gwv1alpha1.VKSRoutePolicy{}, gwhandlers.VKSRoutePolicyToGateway(r.k8sClient)).
		Watches(&v1alpha1.LoadBalancerConfig{}, lbcOwnerToGateway()).
		Watches(&corev1.Service{}, gwhandlers.ServiceToRouteParents(r.k8sClient)).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.maxConcurrentReconciles}).
		Complete(r)
}

// albClassPredicate filters Gateway events to those bound to a GatewayClass
// whose ControllerName is the ALB one. Any other Gateway is dropped before it
// hits the workqueue. We re-check inside the use case as a defence-in-depth.
func albClassPredicate(k8sClient client.Client) predicate.Predicate {
	matches := func(o client.Object) bool {
		gw, ok := o.(*gwv1.Gateway)
		if !ok {
			return false
		}
		if gw.Spec.GatewayClassName == "" {
			return false
		}
		gc := &gwv1.GatewayClass{}
		if err := k8sClient.Get(context.Background(),
			client.ObjectKey{Name: string(gw.Spec.GatewayClassName)}, gc); err != nil {
			return false
		}
		return string(gc.Spec.ControllerName) == consts.GatewayClassControllerNameALB
	}
	return predicate.NewPredicateFuncs(matches)
}
