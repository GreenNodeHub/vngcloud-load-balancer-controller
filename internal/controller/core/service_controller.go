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

package core

import (
	"context"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/core/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	serviceUseCase   usecase.IServiceUseCase
	FinalizerManager k8s.FinalizerManager

	serviceUtils  service.ServiceUtils
	eventRecorder record.EventRecorder
	logger        logr.Logger

	// TODO
	// reconcileCounters *metricsutil.ReconcileCounters
	// metricsCollector  lbcmetrics.MetricCollector

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = contexts.NewContext(ctx).SetLogName("s/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *ServiceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	svc := &corev1.Service{}
	err := r.Client.Get(ctx, req.NamespacedName, svc)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	logger.Info("------------------ START ------------------")
	defer logger.Info("------------------ DONE ------------------")

	if !r.serviceUtils.IsServiceSupported(svc) {
		// in case the service have finalizer but is no longer supported, we still need to call delete to clean up
		// case the service type is changed from LoadBalancer to ClusterIP/NodePort/Headless
		if r.serviceUtils.IsServicePendingFinalization(svc) {
			return r.reconcileDelete(ctx, req, svc)
		}
		return nil
	}

	return r.reconcileEnsure(ctx, req, svc)
}

func (r *ServiceReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	if err := r.FinalizerManager.AddFinalizers(ctx, obj, consts.ServiceFinalizer); err != nil {
		return err
	}
	return r.serviceUseCase.Ensure(ctx, req)
}

func (r *ServiceReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, consts.ServiceFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	if err := r.serviceUseCase.Delete(ctx, req); err != nil {
		return err
	}

	if err := r.FinalizerManager.RemoveFinalizers(ctx, obj, consts.ServiceFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.serviceUtils = service.NewServiceUtils(consts.ServiceFinalizer)
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.eventRecorder,
		r.serviceUtils, r.logger.WithName("eventHandlers").WithName("service"))
	endpointEventHandler := eventhandlers.NewEnqueueRequestForEndpointEvent(r.Client,
		r.logger.WithName("eventHandlers").WithName("endpoint"))
	nodeEventHandler := eventhandlers.NewEnqueueRequestForNodeEvent(r.Client,
		r.logger.WithName("eventHandlers").WithName("node"))

	return ctrl.NewControllerManagedBy(mgr).
		Named("core-service").
		Watches(&corev1.Service{}, svcEventHandler).
		Watches(&corev1.Endpoints{}, endpointEventHandler).
		Watches(&corev1.Node{}, nodeEventHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}
