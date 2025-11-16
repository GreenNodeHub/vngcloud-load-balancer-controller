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

package nsg_controller

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
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/nsg_controller/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/nsg"
)

func NewNodeSecurityGroupReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	nsgUseCase usecase.NodeSecurityGroupUseCase,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	nsgUtils nsg.NodeSecurityGroupUtils,
) *NodeSecurityGroupReconciler {
	return &NodeSecurityGroupReconciler{
		Client:           client,
		Scheme:           scheme,
		nsgUseCase:       nsgUseCase,
		eventRecorder:    eventRecorder,
		finalizerManager: finalizerManager,
		nsgUtils:         nsgUtils,
	}
}

// NodeSecurityGroupReconciler reconciles a NodeSecurityGroup object
type NodeSecurityGroupReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	nsgUseCase       usecase.NodeSecurityGroupUseCase
	eventRecorder    record.EventRecorder
	finalizerManager k8s.FinalizerManager
	nsgUtils         nsg.NodeSecurityGroupUtils

	initDone atomic.Bool
}

// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=nodesecuritygroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=nodesecuritygroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=nodesecuritygroups/finalizers,verbs=update

func (r *NodeSecurityGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if !r.initDone.Load() {
		ctrl.Log.Info("Init not done yet, requeueing...")
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	ctx = contexts.NewContext(ctx).SetLogName("nsg/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *NodeSecurityGroupReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	object := &v1alpha1.NodeSecurityGroup{}
	err := r.Client.Get(ctx, req.NamespacedName, object)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	logger := contexts.NewContext(ctx).Log()
	key := fmt.Sprintf("%s/%s", object.Namespace, object.Name)

	if !r.nsgUtils.IsSupported(object) {
		// in case the service have finalizer but is no longer supported, we still need to call delete to clean up
		// case the service type is changed from LoadBalancer to ClusterIP/NodePort/Headless
		if r.nsgUtils.IsPendingFinalization(object) {
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

func (r *NodeSecurityGroupReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request, obj client.Object) error {
	if err := r.finalizerManager.AddFinalizers(ctx, obj, domain.NsgFinalizer); err != nil {
		return err
	}
	return r.nsgUseCase.EnsureNodeSecurityGroupUseCase(ctx, req)
}

func (r *NodeSecurityGroupReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, obj client.Object) error {
	logger := contexts.NewContext(ctx).Log()
	if !k8s.HasFinalizer(obj, domain.NsgFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	if err := r.nsgUseCase.DeleteNodeSecurityGroupUseCase(ctx, req); err != nil {
		return err
	}

	if err := r.finalizerManager.RemoveFinalizers(ctx, obj, domain.NsgFinalizer); err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeSecurityGroupReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		log := ctrl.Log.WithName("init")
		log.Info("Running initialization...")

		if err := r.nsgUseCase.InitNodeSecurityGroupUseCase(ctx); err != nil {
			log.Error(err, "Fatal: initialization failed")
			return err // returning error causes manager to stop => pod crash
		}

		log.Info("Initialization complete")
		r.initDone.Store(true)
		return nil
	})); err != nil {
		return err
	}

	nsgEventHandler := eventhandlers.NewEnqueueRequestForNsgEvent(r.eventRecorder,
		r.nsgUtils)

	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1alpha1.NodeSecurityGroup{}, nsgEventHandler).
		Named("nodesecuritygroup").
		Complete(r)
}
