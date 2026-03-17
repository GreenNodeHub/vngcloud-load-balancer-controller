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

package service_glb_controller

import (
	"context"
	"fmt"
	"sync/atomic"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/service_glb_controller/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
	service_glb "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service_glb"
)

const (
	controllerName = "service-glb"
)

// NewServiceGLBReconciler creates a new ServiceGLBReconciler.
func NewServiceGLBReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	serviceGLBUseCase usecase.ServiceGLBUseCase,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	serviceGLBUtils service_glb.ServiceGLBUtils,
	metricsCollector lbcmetrics.MetricCollector,
	reconcileCounters *metricsutil.ReconcileCounters,
	maxConcurrentReconciles int,
) *ServiceGLBReconciler {
	return &ServiceGLBReconciler{
		k8sClient:        k8sClient,
		Scheme:           scheme,
		serviceGLBUseCase: serviceGLBUseCase,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		serviceGLBUtils:  serviceGLBUtils,
		annotationParser: annotations.NewSuffixAnnotationParser(domain.GLB_ANNOTATION_PREFIX),

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

// ServiceGLBReconciler reconciles Services with glb.vks.vngcloud.vn/enable=true
type ServiceGLBReconciler struct {
	k8sClient         client.Client
	Scheme            *runtime.Scheme
	serviceGLBUseCase usecase.ServiceGLBUseCase
	finalizerManager  k8s.FinalizerManager
	eventRecorder     record.EventRecorder
	serviceGLBUtils   service_glb.ServiceGLBUtils
	annotationParser  annotations.Parser

	reconcileCounters *metricsutil.ReconcileCounters
	metricsCollector  lbcmetrics.MetricCollector

	maxConcurrentReconciles int

	logger   logr.Logger
	initDone atomic.Bool
}

func (r *ServiceGLBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("Init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	r.reconcileCounters.IncrementService(req.NamespacedName)
	ctx = contexts.NewContext(ctx).SetLogName("svc-glb/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *ServiceGLBReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	svc := &corev1.Service{}
	var err error
	fetchServiceFn := func() {
		err = r.k8sClient.Get(ctx, req.NamespacedName, svc)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_object", fetchServiceFn)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

	if !r.serviceGLBUtils.IsServiceGLBSupported(svc) {
		// Service no longer has the GLB annotation but may still have the finalizer — run delete to clean up
		if r.serviceGLBUtils.IsServiceGLBPendingFinalization(svc) {
			err := r.reconcileDelete(ctx, req, svc)
			if err != nil {
				logger.Errorf("%s Delete failed: %v", domain.ErrorIcon, err)
				r.eventRecorder.Event(svc, corev1.EventTypeWarning, "FailedDelete", err.Error())
				return err
			}
			logger.Infof("%s Delete successfully.", domain.SuccessIcon)
			r.eventRecorder.Event(svc, corev1.EventTypeNormal, "Deleted", key)
			return nil
		}
		return nil
	}

	err = r.reconcileEnsure(ctx, req, svc)
	if err != nil {
		logger.Errorf("%s Ensure failed: %v", domain.ErrorIcon, err)
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, "FailedEnsure", err.Error())
		return err
	}
	logger.Infof("%s Ensure successfully.", domain.SuccessIcon)
	r.eventRecorder.Event(svc, corev1.EventTypeNormal, "Ensured", key)
	return nil
}

func (r *ServiceGLBReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	var err error
	addFinalizersFn := func() {
		err = r.finalizerManager.AddFinalizers(ctx, obj, domain.ServiceGLBFinalizer)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "add_finalizers", addFinalizersFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "add_finalizers_error", err, r.metricsCollector)
	}

	ensureFn := func() {
		err = r.serviceGLBUseCase.EnsureServiceGLBUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "ensure", ensureFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "ensure_error", err, r.metricsCollector)
	}
	return nil
}

func (r *ServiceGLBReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, domain.ServiceGLBFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	var err error
	deleteFn := func() {
		err = r.serviceGLBUseCase.DeleteServiceGLBUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "delete", deleteFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "delete_error", err, r.metricsCollector)
	}

	if err := r.finalizerManager.RemoveFinalizers(ctx, obj, domain.ServiceGLBFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceGLBReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log := ctrl.Log.WithName("init")
		log.Info("Running ServiceGLB initialization...")

		if err := r.serviceGLBUseCase.InitServiceGLBUseCase(ctx); err != nil {
			log.Error(err, "Fatal: ServiceGLB initialization failed")
			return err // returning error causes manager to stop => pod crash
		}

		log.Info("ServiceGLB initialization complete")
		r.initDone.Store(true)
		return nil
	})); err != nil {
		return err
	}

	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceGLBEvent(
		r.eventRecorder,
		r.serviceGLBUtils,
		r.annotationParser,
		r.logger.WithName("eventHandlers").WithName("service"),
	)
	nodeEventHandler := eventhandlers.NewEnqueueRequestForServiceGLBNodeEvent(
		r.k8sClient,
		r.serviceGLBUtils,
		r.logger.WithName("eventHandlers").WithName("node"),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named("service-glb").
		Watches(&corev1.Service{}, svcEventHandler).
		Watches(&corev1.Node{}, nodeEventHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}
