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

package vglb_controller

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

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/vglb_controller/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/vglb"
)

const (
	controllerName = "vglb"
)

func NewVngcloudGlobalLoadBalancerReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	vglbUseCase usecase.VngcloudGlobalLoadBalancerUseCase,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	vglbUtils vglb.VngcloudGlobalLoadBalancerUtils,
	metricsCollector lbcmetrics.MetricCollector,
	reconcileCounters *metricsutil.ReconcileCounters,
	maxConcurrentReconciles int,
) *VngcloudGlobalLoadBalancerReconciler {
	return &VngcloudGlobalLoadBalancerReconciler{
		k8sClient:        k8sClient,
		Scheme:           scheme,
		vglbUseCase:      vglbUseCase,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		vglbUtils:        vglbUtils,

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

// VngcloudGlobalLoadBalancerReconciler reconciles a VngcloudGlobalLoadBalancer object
type VngcloudGlobalLoadBalancerReconciler struct {
	k8sClient        client.Client
	Scheme           *runtime.Scheme
	vglbUseCase      usecase.VngcloudGlobalLoadBalancerUseCase
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	vglbUtils        vglb.VngcloudGlobalLoadBalancerUtils

	reconcileCounters *metricsutil.ReconcileCounters
	metricsCollector  lbcmetrics.MetricCollector

	maxConcurrentReconciles int

	logger   logr.Logger
	initDone atomic.Bool
}

// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers/finalizers,verbs=update

func (r *VngcloudGlobalLoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("Init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	r.reconcileCounters.IncrementVglb(req.NamespacedName)
	ctx = contexts.NewContext(ctx).SetLogName("vglb/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *VngcloudGlobalLoadBalancerReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	object := &v1alpha1.VngcloudGlobalLoadBalancer{}
	var err error
	fetchObjectFn := func() {
		err = r.k8sClient.Get(ctx, req.NamespacedName, object)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_object", fetchObjectFn)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	key := fmt.Sprintf("%s/%s", object.Namespace, object.Name)

	// Check if object is being deleted
	if !object.DeletionTimestamp.IsZero() {
		err := r.reconcileDelete(ctx, req, object)
		if err != nil {
			logger.Errorf("%s Delete failed: %v", domain.ErrorIcon, err)
			r.eventRecorder.Event(object, corev1.EventTypeWarning, "FailedDelete", err.Error())
			return err
		}
		logger.Infof("%s Delete successfully.", domain.SuccessIcon)
		r.eventRecorder.Event(object, corev1.EventTypeNormal, "Deleted", key)
		return nil
	}

	err = r.reconcileEnsure(ctx, req, object)
	if err != nil {
		logger.Errorf("%s Ensure failed: %v", domain.ErrorIcon, err)
		r.eventRecorder.Event(object, corev1.EventTypeWarning, "FailedEnsure", err.Error())
		return err
	}
	logger.Infof("%s Ensure successfully.", domain.SuccessIcon)
	r.eventRecorder.Event(object, corev1.EventTypeNormal, "Ensured", key)
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	var err error
	addFinalizersFn := func() {
		err = r.finalizerManager.AddFinalizers(ctx, obj, domain.VglbFinalizer)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "add_finalizers", addFinalizersFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "add_finalizers_error", err, r.metricsCollector)
	}

	ensureFn := func() {
		err = r.vglbUseCase.EnsureVngcloudGlobalLoadBalancerUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "ensure", ensureFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "ensure_error", err, r.metricsCollector)
	}
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, domain.VglbFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	var err error
	deleteFn := func() {
		err = r.vglbUseCase.DeleteVngcloudGlobalLoadBalancerUseCase(ctx, req)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "delete", deleteFn)
	if err != nil {
		return errs.NewErrorWithMetrics(controllerName, "delete_error", err, r.metricsCollector)
	}

	if err := r.finalizerManager.RemoveFinalizers(ctx, obj, domain.VglbFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VngcloudGlobalLoadBalancerReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log := ctrl.Log.WithName("init")
		log.Info("Running VGLB initialization...")

		if err := r.vglbUseCase.InitVngcloudGlobalLoadBalancerUseCase(ctx); err != nil {
			log.Error(err, "Fatal: VGLB initialization failed")
			return err // returning error causes manager to stop => pod crash
		}

		log.Info("VGLB Initialization complete")
		r.initDone.Store(true)
		return nil
	})); err != nil {
		return err
	}

	vglbEventHandler := eventhandlers.NewEnqueueRequestForVglbEvent(r.eventRecorder, r.vglbUtils, r.logger.WithName("eventHandlers").WithName("vglb"))

	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1alpha1.VngcloudGlobalLoadBalancer{}, vglbEventHandler).
		Named("vngcloudgloballoadbalancer").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}
