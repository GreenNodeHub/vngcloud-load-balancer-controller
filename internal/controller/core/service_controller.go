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
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/core/eventhandlers"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/shared_constants"
)

const (
	serviceTagPrefix        = "service.k8s.aws"
	serviceAnnotationPrefix = "service.beta.kubernetes.io"
	controllerName          = "service"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ServiceController ServiceController

	serviceUtils  service.ServiceUtils
	eventRecorder record.EventRecorder
	logger        logr.Logger

	reconcileCounters *metricsutil.ReconcileCounters
	metricsCollector  lbcmetrics.MetricCollector

	maxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update

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
	r.reconcileCounters.IncrementService(req.NamespacedName)

	ctx = contexts.NewContext(ctx).SetLogName("s/" + req.Namespace + "/" + req.Name).GetContext()
	logger := contexts.NewContext(ctx).Log()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	return errs.HandleReconcileError(r.reconcile(ctx, req), logger)
}

func (r *ServiceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	svc := &corev1.Service{}
	var err error
	fetchServiceFn := func() {
		err = r.Client.Get(ctx, req.NamespacedName, svc)
	}
	r.metricsCollector.ObserveControllerReconcileLatency(controllerName, "fetch_service", fetchServiceFn)
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
			return r.reconcileDelete(ctx, req)
		}
		return nil
	}

	return r.reconcileEnsure(ctx, req)
}

func (r *ServiceReconciler) reconcileEnsure(ctx context.Context, req ctrl.Request) error {
	return r.ServiceController.Ensure(ctx, req)
}

func (r *ServiceReconciler) reconcileDelete(ctx context.Context, req ctrl.Request) error {
	return r.ServiceController.Delete(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	r.serviceUtils = service.NewServiceUtils(shared_constants.ServiceFinalizer)
	svcEventHandler := eventhandlers.NewEnqueueRequestForServiceEvent(r.eventRecorder,
		r.serviceUtils, r.logger.WithName("eventHandlers").WithName("service"))

	return ctrl.NewControllerManagedBy(mgr).
		Named("core-service").
		Watches(&corev1.Service{}, svcEventHandler).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.maxConcurrentReconciles,
		}).
		Complete(r)
}

////////////////////////////////////////////////////////////////////////////////

func NewServiceController() ServiceController {
	return &serviceController{}
}

type ServiceController interface {
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

var _ ServiceController = &serviceController{}

type serviceController struct {
	client.Client
}

func (r *serviceController) Ensure(ctx context.Context, req ctrl.Request) error {
	err := errors.New("not implemented")
	// some errors should not requeue
	if err != nil {
		switch {
		case errs.IsExceededSecurityGroupPerServerQuota(err),
			errs.IsLoadBalancerNotFound(err):
			err = errs.NewNoNeedRequeue(err.Error())
		}
	}
	return nil
}

func (r *serviceController) Delete(ctx context.Context, req ctrl.Request) error {
	return nil
}
