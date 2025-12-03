/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package networking

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/networking/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
)

const (
	controllerName = "ingress"
)

func NewIngressReconciler(
	ingressUseCase usecase.IngressUseCase,
	client client.Client,
	scheme *runtime.Scheme,
	finalizerManager k8s.FinalizerManager,
	eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils,
	metricsCollector lbcmetrics.MetricCollector,
	reconcileCounters *metricsutil.ReconcileCounters,
	maxConcurrentReconciles int,
) *IngressReconciler {
	referenceIndexer := ingress.NewDefaultReferenceIndexer()
	return &IngressReconciler{
		k8sClient:        client,
		Scheme:           scheme,
		ingressUseCase:   ingressUseCase,
		finalizerManager: finalizerManager,
		eventRecorder:    eventRecorder,
		ingressUtils:     ingressUtils,
		referenceIndexer: referenceIndexer,

		logger:            ctrl.Log.WithName("controllers").WithName(controllerName),
		metricsCollector:  metricsCollector,
		reconcileCounters: reconcileCounters,
		maxConcurrentReconciles: func() int {
			if maxConcurrentReconciles > 0 {
				return maxConcurrentReconciles
			}
			return domain.DefaultMaxConcurrentReconciles
		}(),
	}
}

// IngressReconciler reconciles an Ingress object
type IngressReconciler struct {
	k8sClient        client.Client
	Scheme           *runtime.Scheme
	ingressUseCase   usecase.IngressUseCase
	finalizerManager k8s.FinalizerManager

	referenceIndexer ingress.ReferenceIndexer
	ingressUtils     ingress.IngressUtils
	eventRecorder    record.EventRecorder
	logger           logr.Logger

	reconcileCounters *metricsutil.ReconcileCounters
	metricsCollector  lbcmetrics.MetricCollector

	maxConcurrentReconciles int

	initDone atomic.Bool
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("Init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	r.reconcileCounters.IncrementIngress(req.NamespacedName)
	ctx = contexts.NewContext(ctx).SetLogName("ing/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *IngressReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	ing := &networkingv1.Ingress{}
	var err error
	fetchIngressFn := func() {
		err = r.k8sClient.Get(ctx, req.NamespacedName, ing)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_object", fetchIngressFn)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	key := fmt.Sprintf("%s/%s", ing.Namespace, ing.Name)

	if !r.ingressUtils.IsIngressSupported(ing) {
		// in case the ingress have finalizer but is no longer supported, we still need to call delete to clean up
		// case the ingress type is changed from LoadBalancer to ClusterIP/NodePort/Headless
		if r.ingressUtils.IsIngressPendingFinalization(ing) {
			err := r.reconcileDelete(ctx, req, ing)
			if err != nil {
				logger.Errorf("%s Delete failed: %v", domain.ErrorIcon, err)
				r.eventRecorder.Event(ing, corev1.EventTypeWarning, "FailedDelete", err.Error())
				return err
			}
			logger.Infof("%s Delete successfully.", domain.SuccessIcon)
			r.eventRecorder.Event(ing, corev1.EventTypeNormal, "Deleted", key)
			return nil
		}
		return nil
	}

	err = r.reconcileEnsure(ctx, req, ing)
	if err != nil {
		logger.Errorf("%s Ensure failed: %v", domain.ErrorIcon, err)
		r.eventRecorder.Event(ing, corev1.EventTypeWarning, "FailedEnsure", err.Error())
		return err
	}
	logger.Infof("%s Ensure successfully.", domain.SuccessIcon)
	r.eventRecorder.Event(ing, corev1.EventTypeNormal, "Ensured", key)
	return nil
}

func (r *IngressReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	var err error
	addFinalizersFn := func() {
		err = r.finalizerManager.AddFinalizers(ctx, obj, domain.IngressFinalizer)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "add_finalizers", addFinalizersFn)
	if err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return errs.NewErrorWithMetrics(controllerName, "add_finalizers_error", err, r.metricsCollector)
	}

	ensureFn := func() {
		err = r.ingressUseCase.EnsureIngressUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "ensure", ensureFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "ensure_error", err, r.metricsCollector)
	}
	return nil
}

func (r *IngressReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, domain.IngressFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	var err error
	deleteFn := func() {
		err = r.ingressUseCase.DeleteIngressUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "delete", deleteFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "delete_error", err, r.metricsCollector)
	}

	if err := r.finalizerManager.RemoveFinalizers(ctx, obj, domain.IngressFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log := ctrl.Log.WithName("init")
		log.Info("Running initialization...")

		if err := r.ingressUseCase.InitIngressUseCase(ctx); err != nil {
			log.Error(err, "Fatal: initialization failed")
			return err // returning error causes manager to stop => pod crash
		}

		log.Info("Initialization complete")
		r.initDone.Store(true)
		return nil
	})); err != nil {
		return err
	}

	if err := r.setupIndexes(ctx, mgr.GetFieldIndexer()); err != nil {
		return err
	}

	nodeEventHandler := eventhandlers.NewEnqueueRequestForNodeEvent(r.k8sClient,
		r.ingressUtils, r.logger.WithName("eventHandlers").WithName("node"))
	secretEventHandler := eventhandlers.NewEnqueueRequestForSecretEvent(r.k8sClient, r.eventRecorder,
		r.ingressUtils, r.logger.WithName("eventHandlers").WithName("secret"))
	endpointEventHandler := eventhandlers.NewEnqueueRequestForEndpointsEvent(r.k8sClient, r.eventRecorder,
		r.ingressUtils, r.logger.WithName("eventHandlers").WithName("endpoint"))
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.k8sClient, r.eventRecorder,
		r.ingressUtils, r.logger.WithName("eventHandlers").WithName("service"))
	ingEventHandler := eventhandlers.NewEnqueueRequestForIngressEvent(r.eventRecorder,
		r.ingressUtils, r.logger.WithName("eventHandlers").WithName("ingress"))
	lbcEventHandler := eventhandlers.NewEnqueueRequestForLbcEvent(r.k8sClient,
		r.eventRecorder, r.ingressUtils, r.logger.WithName("eventHandlers").WithName("lbc"))

	return ctrl.NewControllerManagedBy(mgr).
		Named("networking-ingress").
		Watches(&networkingv1.Ingress{}, ingEventHandler).
		Watches(&corev1.Service{}, svcEventHandler).
		Watches(&corev1.Endpoints{}, endpointEventHandler).
		Watches(&corev1.Secret{}, secretEventHandler).
		Watches(&corev1.Node{}, nodeEventHandler).
		Watches(&v1alpha1.LoadBalancerConfig{}, lbcEventHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}

func (r *IngressReconciler) setupIndexes(ctx context.Context, fieldIndexer client.FieldIndexer) error {
	if err := fieldIndexer.IndexField(ctx, &networkingv1.Ingress{}, ingress.IndexKeyServiceRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildServiceRefIndexes(context.Background(), obj.(*networkingv1.Ingress))
		},
	); err != nil {
		return err
	}
	if err := fieldIndexer.IndexField(ctx, &networkingv1.Ingress{}, ingress.IndexKeySecretRefName,
		func(obj client.Object) []string {
			return r.referenceIndexer.BuildSecretRefIndexes(context.Background(), obj.(*networkingv1.Ingress))
		},
	); err != nil {
		return err
	}
	return nil
}
