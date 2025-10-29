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

package controller

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/vlbc"
)

func NewVngcloudLoadBalancerConfigReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	vlbcUseCase usecase.IVLBConfigUseCase,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	vlbcUtils vlbc.VLBCUtils,
) *VngcloudLoadBalancerConfigReconciler {
	return &VngcloudLoadBalancerConfigReconciler{
		Client:           client,
		Scheme:           scheme,
		vlbcUseCase:      vlbcUseCase,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		vlbcUtils:        vlbcUtils,
	}
}

// VngcloudLoadBalancerConfigReconciler reconciles a VngcloudLoadBalancerConfig object
type VngcloudLoadBalancerConfigReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	vlbcUseCase      usecase.IVLBConfigUseCase
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	vlbcUtils        vlbc.VLBCUtils

	initDone atomic.Bool
}

// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudloadbalancerconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudloadbalancerconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudloadbalancerconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VngcloudLoadBalancerConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *VngcloudLoadBalancerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("Init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	ctx = contexts.NewContext(ctx).SetLogName("vlbc/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *VngcloudLoadBalancerConfigReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	object := &v1alpha1.VngcloudLoadBalancerConfig{}
	err := r.Client.Get(ctx, req.NamespacedName, object)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	key := fmt.Sprintf("%s/%s", object.Namespace, object.Name)

	if !r.vlbcUtils.IsSupported(object) {
		// in case the service have finalizer but is no longer supported, we still need to call delete to clean up
		// case the service type is changed from LoadBalancer to ClusterIP/NodePort/Headless
		if r.vlbcUtils.IsPendingFinalization(object) {
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

func (r *VngcloudLoadBalancerConfigReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	if err := r.finalizerManager.AddFinalizers(ctx, obj, consts.VLBCFinalizer); err != nil {
		return err
	}
	return r.vlbcUseCase.Ensure(ctx, req)
}

func (r *VngcloudLoadBalancerConfigReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, consts.VLBCFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	if err := r.vlbcUseCase.Delete(ctx, req); err != nil {
		return err
	}

	if err := r.finalizerManager.RemoveFinalizers(ctx, obj, consts.VLBCFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VngcloudLoadBalancerConfigReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log := ctrl.Log.WithName("init")
		log.Info("Running initialization...")

		if err := r.vlbcUseCase.Init(ctx); err != nil {
			log.Error(err, "Fatal: initialization failed")
			return err // returning error causes manager to stop => pod crash
		}

		log.Info("Initialization complete")
		r.initDone.Store(true)
		return nil
	})); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VngcloudLoadBalancerConfig{}).
		Named("vngcloudloadbalancerconfig").
		Complete(r)
}
