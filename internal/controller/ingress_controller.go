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
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Config           *Config
	FinalizerManager k8s.FinalizerManager

	eventClassification *EventClassification
	annotationParser    annotations.Parser
	resourceDependant   ResourceDependant[*networkingv1.Ingress]

	modeTest   bool
	ensureTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	deleteTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)

	// at the first time, we should ignore event create Node
	isShouldReconcile bool
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

func (r *IngressReconciler) isValid(obj interface{}) bool {
	object, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return false
	}
	if *object.Spec.IngressClassName != consts.IngressClass {
		return false
	}
	if !object.GetDeletionTimestamp().IsZero() {
		return false
	}
	return true
}

func (r *IngressReconciler) getObjectByKey(key string) (interface{}, bool) {
	namespace, name := revertKey(key)
	resource := &networkingv1.Ingress{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, resource)
	if err != nil {
		return nil, false
	}
	return resource, true
}

// getReconcileRequests returns a list of reconcile requests periodically (which LoadBalancer have changed and need to be reconciled)
func (r *IngressReconciler) getReconcileRequestsPeriodically() []reconcile.Request {
	return []reconcile.Request{}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := genKey(req.Namespace, req.Name)

	ctx = contexts.NewContext(ctx).SetLogName(fmt.Sprint("i/" + key)).GetContext()
	logger := contexts.NewContext(ctx).Log()

	event := r.eventClassification.Classify(key)
	if event == nil || event.Obj == nil {
		logger.Info("Event is nil, return.")
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
		return r.deleteObject(ctx, event.Obj.(*networkingv1.Ingress))
	default:
		return r.ensureObject(ctx, event.Obj.(*networkingv1.Ingress))
	}
}

func (r *IngressReconciler) ensureObject(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()

	if err := r.FinalizerManager.AddFinalizers(ctx, ingress, consts.IngressFinalizer); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Reconcile Object ", genKey(ingress.Namespace, ingress.Name))
	time.Sleep(3 * time.Second)
	logger.Info("Done Reconcile Object")

	r.resourceDependant.Set(ingress, true)
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) deleteObject(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()

	r.resourceDependant.Clear(ingress)

	if k8s.HasFinalizer(ingress, consts.IngressFinalizer) {
		logger.Info("Delete Object ", genKey(ingress.Namespace, ingress.Name))
		time.Sleep(10 * time.Second)
		logger.Info("Done Delete Object")

		if err := r.FinalizerManager.RemoveFinalizers(ctx, ingress, consts.IngressFinalizer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.eventClassification = NewEventClassification(r.getObjectByKey, r.isValid)
	r.annotationParser = annotations.NewSuffixAnnotationParser(consts.INGRESS_ANNOTATION_PREFIX)
	r.resourceDependant = NewIngressDependant(r.Client)

	// periodicReconciler := NewPeriodicReconciler(r, 1*time.Second, r.getReconcileRequestsPeriodically)
	// ctx := context.Background()
	// periodicReconciler.Start(ctx)

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Watches(
			&corev1.Endpoints{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, endpoint client.Object) []reconcile.Request {
				return r.resourceDependant.GetResourceNeedReconcile("endpoint", endpoint.GetNamespace(), endpoint.GetName())
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, service client.Object) []reconcile.Request {
				return r.resourceDependant.GetResourceNeedReconcile("service", service.GetNamespace(), service.GetName())
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
				// list all ingresses
				ingresses := &networkingv1.IngressList{}
				err := r.List(ctx, ingresses)
				if err != nil {
					return []reconcile.Request{}
				}

				// filter ingresses with class
				requests := make([]reconcile.Request, 0)
				for _, ingress := range ingresses.Items {
					if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == consts.IngressClass {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      ingress.GetName(),
								Namespace: ingress.GetNamespace(),
							},
						})
					}
				}
				return requests
			}),
			builder.WithPredicates(predicate.AnnotationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5, // ..................
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				switch e.Object.(type) {
				case *networkingv1.Ingress:
					if object := e.Object.(*networkingv1.Ingress); object.Spec.IngressClassName == nil ||
						*object.Spec.IngressClassName != consts.IngressClass {
						return false
					}
					logrus.Info("Create Ingress: ")
					return true
				case *corev1.Service:
					return false
				case *corev1.Endpoints:
					return false
				case *corev1.Node:
					if !r.isShouldReconcile {
						r.isShouldReconcile = true
						return false
					}
					logrus.Info("Create Node: ")
					return true
				default:
					logrus.Warn("object is of an unknown type: ", e.Object)
					return false
				}
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				switch e.ObjectNew.(type) {
				case *networkingv1.Ingress:
					oldIngress := e.ObjectOld.(*networkingv1.Ingress)
					newIngress := e.ObjectNew.(*networkingv1.Ingress)
					if (oldIngress.Spec.IngressClassName == nil || *oldIngress.Spec.IngressClassName != consts.IngressClass) &&
						(newIngress.Spec.IngressClassName == nil || *newIngress.Spec.IngressClassName != consts.IngressClass) {
						return false
					}
					if !reflect.DeepEqual(oldIngress.Spec, newIngress.Spec) {
						logrus.Info("Spec changed")
						return true
					}

					// remove whitelisted annotations in the comparison
					for k := range consts.WhitelistedAnnotations {
						delete(oldIngress.Annotations, k)
						delete(newIngress.Annotations, k)
					}
					if !reflect.DeepEqual(oldIngress.Annotations, newIngress.Annotations) {
						logrus.Info("Annotations changed")
						return true
					}

					if !reflect.DeepEqual(oldIngress.DeletionTimestamp.IsZero(), newIngress.DeletionTimestamp.IsZero()) {
						logrus.Info("DeletionTimestamp changed")
						return true
					}
					return false

				case *corev1.Service:
					logrus.Info("Update Service: ")
					return true

				case *corev1.Endpoints:
					logrus.Info("Update Endpoints: ")
					return true
				case *corev1.Node:
					oldIngress := e.ObjectOld.(*corev1.Node)
					newIngress := e.ObjectNew.(*corev1.Node)
					if oldIngress.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] != newIngress.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] {
						logrus.Info("Node Annotations changed")
						return true
					}
					return false
				default:
					logrus.Warn("object is of an unknown type: ", e.ObjectNew)
					return false
				}
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				switch e.Object.(type) {
				case *networkingv1.Ingress:
					if object := e.Object.(*networkingv1.Ingress); object.Spec.IngressClassName == nil ||
						*object.Spec.IngressClassName != consts.IngressClass {
						return false
					}
					logrus.Info("Delete Ingress: ")
					return true
				case *corev1.Service:
					logrus.Info("Delete Service: ")
					return true
				case *corev1.Endpoints:
					logrus.Info("Delete Endpoints: ")
					return true
				case *corev1.Node:
					logrus.Info("Delete Node: ")
					return true
				default:
					logrus.Warn("object is of an unknown type: ", e.Object)
					return false
				}
			},
		}).
		Complete(r)
}
