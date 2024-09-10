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
	"reflect"
	"strings"
	"time"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	FinalizerManager k8s.FinalizerManager

	eventClassification *EventClassification

	modeTest   bool
	ensureTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	deleteTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update

// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=endpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=endpoints/finalizers,verbs=update

// +kubebuilder:rbac:groups="",resources=node,verbs=get;list;watch
func genKey(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}
func revertKey(key string) (string, string) {
	split := strings.Split(key, "/")
	return split[0], split[1]
}
func (r *ServiceReconciler) isValid(obj interface{}) bool {
	object, ok := obj.(*corev1.Service)
	if !ok {
		return false
	}
	if object.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	if !object.GetDeletionTimestamp().IsZero() {
		return false
	}
	return true
}

func (r *ServiceReconciler) getObjectByKey(key string) (interface{}, bool) {
	namespace, name := revertKey(key)
	resource := &corev1.Service{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, resource)
	if err != nil {
		return nil, false
	}
	return resource, true
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	key := genKey(req.Namespace, req.Name)
	event := r.eventClassification.Classify(key)
	if event == nil || event.Obj == nil {
		klog.Info("Event is nil, return.")
		return ctrl.Result{}, nil
	}

	// for testing
	if r.modeTest {
		switch event.Type {
		case DeleteEvent:
			return r.deleteTest(ctx, req)
		default:
			return r.ensureTest(ctx, req)
		}
	}

	switch event.Type {
	case DeleteEvent:
		return r.deleteObject(ctx, event.Obj.(*corev1.Service))
	default:
		return r.ensureObject(ctx, event.Obj.(*corev1.Service))
	}
}

func (r *ServiceReconciler) ensureObject(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	if err := r.FinalizerManager.AddFinalizers(ctx, svc, consts.ServiceFinalizer); err != nil {
		// r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrl.Result{}, err
	}

	klog.Info("Reconcile Object ", genKey(svc.Namespace, svc.Name))
	time.Sleep(3 * time.Second)
	klog.Info("Done Reconcile Object")
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) deleteObject(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	if k8s.HasFinalizer(svc, consts.ServiceFinalizer) {
		klog.Info("Delete Object ", genKey(svc.Namespace, svc.Name))
		time.Sleep(10 * time.Second)
		klog.Info("Done Delete Object")

		if err := r.FinalizerManager.RemoveFinalizers(ctx, svc, consts.ServiceFinalizer); err != nil {
			// r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.eventClassification = NewEventClassification(r.getObjectByKey, r.isValid)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Watches(
			&corev1.Endpoints{},
			handler.EnqueueRequestsFromMapFunc(r.findServiceForEndpoints),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.findServiceForNode),
			builder.WithPredicates(predicate.AnnotationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5, // ..................
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				switch e.Object.(type) {
				case *corev1.Service:
					if object := e.Object.(*corev1.Service); object.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}
					klog.Info("Create Service: ")
					return true
				case *corev1.Endpoints:
					return false
				case *corev1.Node:
					klog.Info("Create Node: ")
					return true
				default:
					klog.Info("object is of an unknown type: ", e.Object)
					return false
				}
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				switch e.ObjectNew.(type) {
				case *corev1.Service:
					oldIngress := e.ObjectOld.(*corev1.Service)
					newIngress := e.ObjectNew.(*corev1.Service)

					if oldIngress.Spec.Type != corev1.ServiceTypeLoadBalancer && newIngress.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}

					if !reflect.DeepEqual(oldIngress.Spec, newIngress.Spec) {
						klog.Info("Service Spec changed")
						return true
					}

					// remove whitelisted annotations in the comparison
					for k := range consts.WhitelistedAnnotations {
						delete(oldIngress.Annotations, k)
						delete(newIngress.Annotations, k)
					}
					if !reflect.DeepEqual(oldIngress.Annotations, newIngress.Annotations) {
						klog.Info("Service Annotations changed")
						return true
					}
					if !reflect.DeepEqual(oldIngress.DeletionTimestamp.IsZero(), newIngress.DeletionTimestamp.IsZero()) {
						klog.Info("Service DeletionTimestamp changed")
						return true
					}
					return false

				case *corev1.Endpoints:
					klog.Info("Update Endpoints: ")
					return true
				case *corev1.Node:
					oldIngress := e.ObjectOld.(*corev1.Node)
					newIngress := e.ObjectNew.(*corev1.Node)
					if oldIngress.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] != newIngress.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] {
						klog.Info("Node Annotations changed")
						return true
					}
					return false
				default:
					klog.Info("object is of an unknown type: ", e.ObjectNew)
					return false
				}
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				switch e.Object.(type) {
				case *corev1.Service:
					if object := e.Object.(*corev1.Service); object.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}
					klog.Info("Delete Service: ")
					return true
				case *corev1.Endpoints:
					klog.Info("Delete Endpoints: ")
					return true
				case *corev1.Node:
					klog.Info("Delete Node: ")
					return true
				default:
					klog.Info("object is of an unknown type: ", e.Object)
					return false
				}
			},
		}).
		Complete(r)
}

func (r *ServiceReconciler) findServiceForEndpoints(ctx context.Context, endpoint client.Object) []reconcile.Request {
	serviceSelect := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: endpoint.GetName(), Namespace: endpoint.GetNamespace()}, serviceSelect)
	if err != nil {
		return []reconcile.Request{}
	}

	if serviceSelect.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return []reconcile.Request{}
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      serviceSelect.GetName(),
			Namespace: serviceSelect.GetNamespace(),
		},
	}}
}

func (r *ServiceReconciler) findServiceForNode(ctx context.Context, _ client.Object) []reconcile.Request {
	// list all services
	services := &corev1.ServiceList{}
	err := r.List(ctx, services)
	if err != nil {
		return []reconcile.Request{}
	}

	// filter services with type LoadBalancer
	var loadBalancerServices []corev1.Service
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			loadBalancerServices = append(loadBalancerServices, service)
		}
	}

	// create requests for all load balancer services
	requests := make([]reconcile.Request, len(loadBalancerServices))
	for i, service := range loadBalancerServices {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: service.GetName(),
			},
		}
	}
	return requests
}
