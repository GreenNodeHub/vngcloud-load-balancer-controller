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
	"time"

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/event_classification"
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-fleet-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

// VngcloudGlobalLoadBalancerReconciler reconciles a VngcloudGlobalLoadBalancer object
type VngcloudGlobalLoadBalancerReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	Config           *config.Config
	FinalizerManager k8s.FinalizerManager

	eventClassification *event_classification.EventClassification
	Provider            provider.Provider
	// annotationParser    annotations.Parser
}

func (r *VngcloudGlobalLoadBalancerReconciler) init() error {
	r.eventClassification = event_classification.NewEventClassification(r.getObjectByKey, r.isValid)
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) isValid(obj client.Object) bool {
	_, ok := obj.(*v1alpha1.VngcloudGlobalLoadBalancer)
	return ok
}

func (r *VngcloudGlobalLoadBalancerReconciler) getObjectByKey(key string) (client.Object, bool) {
	namespace, name := revertKey(key)
	resource := &v1alpha1.VngcloudGlobalLoadBalancer{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, resource)
	if err != nil {
		return nil, false
	}
	return resource, true
}

// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vks.vngcloud.vn,resources=vngcloudgloballoadbalancers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VngcloudGlobalLoadBalancer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *VngcloudGlobalLoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = contexts.NewContext(ctx).SetLogName(fmt.Sprint("glb/" + genKey(req.Namespace, req.Name))).GetContext()
	logger := contexts.NewContext(ctx).Log()
	logger.Info("------------------ START ------------------")
	defer logger.Info("------------------ DONE ------------------")
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	err := r.reconcile(ctx, req)
	return errs.HandleReconcileError(err, logger)
}

func (r *VngcloudGlobalLoadBalancerReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	logger := contexts.NewContext(ctx).Log()
	key := genKey(req.Namespace, req.Name)

	event := r.eventClassification.Classify(key)
	if event == nil {
		logger.Info("Event is nil, return.")
		return nil
	} else if event.Obj == nil {
		logger.Infof("Event=%v but object is nil, return.", event.Type)
		return nil
	}
	logger.Infof("Event = %v", event.Type)

	switch event.Type {
	case event_classification.DeleteEvent:
		obj := event.Obj.(*v1alpha1.VngcloudGlobalLoadBalancer)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Deleting", key)
		err := r.deleteObject(ctx, obj)
		if err == nil {
			logger.Infof("%s Delete successfully.", successIcon)
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Deleted", key)
		} else {
			logger.Errorf("%s Delete failed: %v", errorIcon, err)
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedDelete", err.Error())
		}
		return err
	case event_classification.CreateEvent:
		obj := event.Obj.(*v1alpha1.VngcloudGlobalLoadBalancer)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Creating", key)
		err := r.ensureObject(ctx, obj, nil)
		if err == nil {
			logger.Infof("%s Create successfully.", successIcon)
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Created", key)
		} else {
			logger.Errorf("%s Create failed: %v", errorIcon, err)
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedCreate", err.Error())
		}
		return err
	default:
		obj := event.Obj.(*v1alpha1.VngcloudGlobalLoadBalancer)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Updating", key)
		err := r.ensureObject(ctx, obj, event.OldObj)
		if err == nil {
			logger.Infof("%s Update successfully.", successIcon)
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Updated", key)
		} else {
			logger.Errorf("%s Update failed: %v", errorIcon, err)
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedUpdate", err.Error())
		}
		return err
	}
}

func (r *VngcloudGlobalLoadBalancerReconciler) ensureObject(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer, oldObjInterface interface{}) error {
	logger := contexts.NewContext(ctx).Log()

	if err := r.FinalizerManager.AddFinalizers(ctx, obj, consts.GLBFinalizer); err != nil {
		return err
	}

	logger.Info("Ensuring object...")

	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) deleteObject(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer) error {
	logger := contexts.NewContext(ctx).Log()

	if !k8s.HasFinalizer(obj, consts.GLBFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	err := r.subDeleteObject(ctx, obj)
	if err != nil {
		return err
	}

	if err := r.FinalizerManager.RemoveFinalizers(ctx, obj, consts.GLBFinalizer); err != nil {
		return err
	}
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) subDeleteObject(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer) error {
	logger := contexts.NewContext(ctx).Log()

	logger.Info("Deleting object...")

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VngcloudGlobalLoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := r.init()
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1alpha1.VngcloudGlobalLoadBalancer{}).
		Complete(r)
}
