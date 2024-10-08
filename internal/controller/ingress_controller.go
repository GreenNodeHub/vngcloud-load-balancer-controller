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
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sBuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/builder"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/event_classification"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	periodicreconciler "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/periodic_reconciler"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Config           *config.Config
	FinalizerManager k8s.FinalizerManager

	eventClassification *event_classification.EventClassification
	annotationParser    annotations.Parser
	resourceDependant   ResourceDependant[*networkingv1.Ingress]

	Provider provider.Provider

	// for testing reconcile behavior when some resource is created, updated, deleted
	modeTest   bool
	ensureTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	deleteTest func(ctx context.Context, req ctrl.Request) (ctrl.Result, error)

	netwotkID  string
	subnetID   string
	subnetCIDR string

	knownNodes []*corev1.Node

	// store to delete redundant loadbalancer resources
	cacheLoadBalancerBuilder map[string]builder.LoadbalancerBuilder

	//  flag to check if the reconciler is initialized
	initialized bool
	initLock    sync.Mutex
}

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

func (r *IngressReconciler) getNodes(client client.Client) ([]*corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := client.List(context.Background(), nodes)
	if err != nil {
		return nil, err
	}
	// filter nodes with annotation
	filteredNodes := make([]*corev1.Node, 0)
	for i := range nodes.Items {
		if nodes.Items[i].Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] != "true" {
			filteredNodes = append(filteredNodes, &nodes.Items[i])
		}
	}
	return filteredNodes, nil
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

func (r *IngressReconciler) updateObjectAnnotation(ctx context.Context, obj *networkingv1.Ingress, key, value string) error {
	if obj == nil {
		return nil
	}
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}
	obj.Annotations[key] = value
	return r.Update(ctx, obj, &client.UpdateOptions{})
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
	if !r.initialized {
		return ctrl.Result{Requeue: true}, nil
	}
	key := genKey(req.Namespace, req.Name)

	ctx = contexts.NewContext(ctx).SetLogName(fmt.Sprint("i/" + key)).GetContext()
	logger := contexts.NewContext(ctx).Log()
	defer logger.Info("------------------------------------")

	event := r.eventClassification.Classify(key)
	if event == nil || event.Obj == nil {
		logger.Info("Event is nil, return.")
		return ctrl.Result{}, nil
	}

	// for testing
	if r.modeTest {
		switch event.Type {
		case event_classification.DeleteEvent:
			r.resourceDependant.Clear(event.Obj.(*networkingv1.Ingress))
			return r.deleteTest(ctx, req)
		default:
			r.resourceDependant.Set(event.Obj.(*networkingv1.Ingress), true)
			return r.ensureTest(ctx, req)
		}
	}

	switch event.Type {
	case event_classification.DeleteEvent:
		return r.deleteObject(ctx, event.Obj.(*networkingv1.Ingress))
	case event_classification.CreateEvent:
		return r.ensureObject(ctx, event.Obj.(*networkingv1.Ingress), event.OldObj)
	default:
		return r.ensureObject(ctx, event.Obj.(*networkingv1.Ingress), event.OldObj)
	}
}

func (r *IngressReconciler) ensureObject(ctx context.Context, obj *networkingv1.Ingress, oldObjInterface interface{}) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()

	if err := r.FinalizerManager.AddFinalizers(ctx, obj, consts.IngressFinalizer); err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrl.Result{}, err
	}

	logger.Info("Reconcile Object ", genKey(obj.Namespace, obj.Name))

	loadBalancerBuilder, err := builder.NewLoadBalancerBuilderByIngress(ctx, obj, r.annotationParser, r.Client,
		r.netwotkID, r.subnetID, r.subnetCIDR,
		r.Config.Cluster.ClusterID,
		r.knownNodes,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// logger.Info("LoadBalancerBuilder: ", loadBalancerBuilder.StringFormat(), " ", loadBalancerBuilder.JSONFormat())
	// loadBalancerBuilder.Print()

	// ignore reconcile
	if loadBalancerBuilder.IsIgnored() {
		logger.Info("Object is ignored")
		return ctrl.Result{}, nil
	}

	// create loadbalancer, update annotation and reconcile later
	if loadBalancerBuilder.GetID() == "" {
		// check if loadbalancer with the generate name exists, if exists, update annotation and return
		lb, err := r.Provider.GetLoadBalancerByName(loadBalancerBuilder.GetName())
		if err != nil {
			return ctrl.Result{}, err
		}
		if lb == nil {
			// create loadbalancer. It mays create lb, listener, pool at the same time
			lb, err = r.Provider.CreateLoadBalancer(loadBalancerBuilder.CreateLoadBalancerOptions())
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if lb == nil || lb.UUID == "" {
			return ctrl.Result{}, errs.ErrorLoadBalancerNotHaveUUID
		}
		r.updateObjectAnnotation(ctx, obj, fmt.Sprintf("%s/%s", consts.INGRESS_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID), lb.UUID)
		return ctrl.Result{}, nil
	} else {
		if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return ctrl.Result{}, err
		}
	}

	// inspect current loadbalancer in portal to compare with the new one
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, loadBalancerBuilder.GetID(), r.Provider)
	if err != nil {
		logger.Error("Failed to get current loadbalancer: ", err)
		return ctrl.Result{}, err
	}

	// build oldBuilder
	var (
		oldBuilder builder.LoadbalancerBuilder
		ok         bool
	)
	if oldBuilder, ok = r.cacheLoadBalancerBuilder[genKey(obj.Namespace, obj.Name)]; !ok {
		// check oldObjInterface can be converted to *networkingv1.Ingress, if not, set oldObj = nil
		var oldObj *networkingv1.Ingress
		if oldObj, ok = oldObjInterface.(*networkingv1.Ingress); !ok {
			oldObj = nil
		}
		// build again
		var err error
		oldBuilder, err = builder.NewLoadBalancerBuilderByIngress(ctx, oldObj, r.annotationParser, r.Client,
			r.netwotkID, r.subnetID, r.subnetCIDR,
			r.Config.Cluster.ClusterID,
			r.knownNodes,
		)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// // ensure tags
	// tags := VNGHelper.MergeTags(ctx, currentBuilder, loadBalancerBuilder)
	// if tags != nil {
	// 	if err := r.Provider.UpdateTags(loadBalancerBuilder.GetID(), tags); err != nil {
	// 		logger.Error("Failed to update tags: ", err)
	// 		return ctrl.Result{}, err
	// 	}
	// }

	// ensure package
	if currentBuilder.GetPackageID() != loadBalancerBuilder.GetPackageID() &&
		currentBuilder.GetPackageID() != "" &&
		loadBalancerBuilder.GetPackageID() != "" {
		if err := r.Provider.ResizeLoadBalancer(loadBalancerBuilder.GetID(), loadBalancerBuilder.GetPackageID()); err != nil {
			logger.Error("Failed to resize loadbalancer: ", err)
			return ctrl.Result{}, err
		}
		if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return ctrl.Result{}, err
		}
	}

	// ensure pools
	for _, poolBuilder := range loadBalancerBuilder.GetPoolBuilders() {
		poolInPortal := currentBuilder.GetPoolBuilderByName(poolBuilder.GetName())
		if poolInPortal == nil {
			if _pool, err := r.Provider.CreatePool(loadBalancerBuilder.GetID(),
				poolBuilder.GetICreatePoolRequest(loadBalancerBuilder.GetID())); err != nil {
				logger.Error("Failed to create pool: ", err)
				return ctrl.Result{}, err
			} else {
				poolBuilder.SetID(_pool.UUID)
			}

			if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
		} else {
			poolBuilder.SetID(poolInPortal.GetID())
			updateOptions, message := builder.VNGHelper.ComparePoolBuilder(loadBalancerBuilder.GetID(), poolInPortal, poolBuilder)
			if updateOptions != nil {
				logger.Info("Need update pool: ", strings.Join(message, ", "))
				err := r.Provider.UpdatePool(loadBalancerBuilder.GetID(), poolInPortal.GetID(),
					updateOptions.WithLoadBalancerId(loadBalancerBuilder.GetID()))
				if err != nil {
					logger.Error("Failed to update pool: ", err)
					return ctrl.Result{}, err
				}
				if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			}

			// ensure pool members, with default pool, should merge pool members, otherwise, should update
			if poolBuilder.GetName() == consts.DEFAULT_NAME_DEFAULT_POOL {
				updateOptions, err := builder.VNGHelper.MergePoolMembers(loadBalancerBuilder.GetID(),
					currentBuilder.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL),
					oldBuilder.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL),
					loadBalancerBuilder.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL))
				if err != nil {
					logger.Error("Failed to merge pool members: ", err)
					return ctrl.Result{}, err
				}
				if updateOptions == nil {
					continue
				}
				err = r.Provider.UpdatePoolMembers(loadBalancerBuilder.GetID(), poolInPortal.GetID(),
					updateOptions)
				if err != nil {
					logger.Error("Failed to update pool members: ", err)
					return ctrl.Result{}, err
				}
				if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			} else // normal pool
			if !builder.VNGHelper.ComparePoolMembers(poolInPortal.Members, poolBuilder.Members, true) {
				err := r.Provider.UpdatePoolMembers(loadBalancerBuilder.GetID(), poolInPortal.GetID(),
					poolBuilder.GetIUpdatePoolMembersRequest(loadBalancerBuilder.GetID()))
				if err != nil {
					logger.Error("Failed to update pool members: ", err)
					return ctrl.Result{}, err
				}
				if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			}
		}
	}

	// ensure listeners
	isHavingDefaultPool := false
	defaultPoolBuilder := loadBalancerBuilder.GetPoolBuilderByName(consts.DEFAULT_NAME_DEFAULT_POOL)
	if defaultPoolBuilder != nil {
		isHavingDefaultPool = true
	}
	for _, listenerBuilder := range loadBalancerBuilder.GetListenerBuilders() {
		// set default pool id for listener
		if isHavingDefaultPool {
			listenerBuilder.SetPoolID(defaultPoolBuilder.GetID())
		}

		// listenerInPortal := currentBuilder.GetListenerBuilderByName(listenerBuilder.GetName())
		listenerInPortal := currentBuilder.GetListenerBuilderByPort(listenerBuilder.ListenerProtocolPort)
		if listenerInPortal == nil {
			if _, err := r.Provider.CreateListener(loadBalancerBuilder.GetID(),
				listenerBuilder.GetICreateListenerRequest().WithLoadBalancerId(loadBalancerBuilder.GetID()),
			); err != nil {
				logger.Error("Failed to create listener: ", err)
				return ctrl.Result{}, err
			}
			if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
		} else {
			listenerBuilder.SetID(listenerInPortal.GetID())

			// if mismatch listener protocol, return error => user must delete listener in portal ..........................
			if listenerInPortal.ListenerProtocol != listenerBuilder.ListenerProtocol {
				logger.Error("Listener protocol mismatch: ", listenerInPortal.ListenerProtocol, listenerBuilder.ListenerProtocol)
				return ctrl.Result{}, nil
			}

			updateOptions, message := builder.VNGHelper.CompareListenerBuilder(loadBalancerBuilder.GetID(), listenerInPortal, listenerBuilder)
			if updateOptions != nil {
				logger.Info("Need update listener: ", strings.Join(message, ", "))
				err := r.Provider.UpdateListener(loadBalancerBuilder.GetID(), listenerInPortal.GetID(), updateOptions)
				if err != nil {
					logger.Error("Failed to update listener: ", err)
					return ctrl.Result{}, err
				}

				// need to update to current builder, avoid mismatch data later
				listenerInPortal.DefaultPoolId = &updateOptions.DefaultPoolId
				listenerInPortal.ReferPoolName = ""
				if p := loadBalancerBuilder.GetPoolBuilderByID(updateOptions.DefaultPoolId); p != nil {
					listenerInPortal.ReferPoolName = p.GetName()
				}
				if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			}
		}

		// ensure policy
		for _, policyBuilder := range listenerBuilder.GetPolicyBuilders() {
			// set policy redirect to pool id if exists
			if policyBuilder.ReferPoolName != "" {
				referPool := loadBalancerBuilder.GetPoolBuilderByName(policyBuilder.ReferPoolName)
				if referPool == nil {
					logger.Error("Failed to get refer pool: ", listenerBuilder.GetPoolName())
					return ctrl.Result{}, nil
				}
				policyBuilder.RedirectPoolID = referPool.GetID()
			}

			policyInPortal := listenerInPortal.GetPolicyBuilderByName(policyBuilder.GetName())
			if policyInPortal == nil {
				if _, err := r.Provider.CreatePolicy(loadBalancerBuilder.GetID(), listenerInPortal.GetID(),
					policyBuilder.GetICreatePolicyRequest(loadBalancerBuilder.GetID(), listenerInPortal.GetID())); err != nil {
					logger.Error("Failed to create policy: ", err)
					return ctrl.Result{}, err
				}
				if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			} else {
				policyBuilder.SetID(policyInPortal.GetID())
				updateOptions, message := builder.VNGHelper.ComparePolicyBuilder(loadBalancerBuilder.GetID(), listenerBuilder.GetID(),
					policyInPortal, policyBuilder)
				if updateOptions != nil {
					logger.Info("Need update policy: ", strings.Join(message, ", "))
					err := r.Provider.UpdatePolicy(loadBalancerBuilder.GetID(), listenerBuilder.GetID(), policyInPortal.GetID(), updateOptions)
					if err != nil {
						logger.Error("Failed to update policy: ", err)
						return ctrl.Result{}, err
					}

					// need to update to current builder, avoid mismatch data later
					listenerInPortal.DefaultPoolId = &updateOptions.RedirectPoolID
					listenerInPortal.ReferPoolName = ""
					if p := loadBalancerBuilder.GetPoolBuilderByID(updateOptions.RedirectPoolID); p != nil {
						listenerInPortal.ReferPoolName = p.GetName()
					}
					if _, err := r.Provider.WaitForLBActive(loadBalancerBuilder.GetID()); err != nil {
						logger.Error("Failed to wait for loadbalancer active: ", err)
						return ctrl.Result{}, err
					}
				}
			}
		}
	}

	// delete redundant listeners
	for _, oldListener := range oldBuilder.GetListenerBuilders() {
		currentListener := currentBuilder.GetListenerBuilderByName(oldListener.GetName())
		newListener := loadBalancerBuilder.GetListenerBuilderByName(oldListener.GetName())
		if currentListener != nil && newListener == nil {
			if err := r.Provider.DeleteListener(currentBuilder.GetID(), currentListener.GetID()); err != nil {
				logger.Error("Failed to delete listener: ", err)
				return ctrl.Result{}, err
			}
			if _, err := r.Provider.WaitForLBActive(currentBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
			currentListener.SetIsDeleted(true)
			continue
		}

		// delete redundant policy
		for _, policy := range oldListener.GetPolicyBuilders() {
			if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy != nil &&
				newListener.GetPolicyBuilderByName(policy.GetName()) == nil {
				if err := r.Provider.DeletePolicy(currentBuilder.GetID(), currentListener.GetID(), currentPolicy.GetID()); err != nil {
					logger.Error("Failed to delete policy: ", err)
					return ctrl.Result{}, err
				}
				currentPolicy.SetIsDeleted(true)
				if _, err := r.Provider.WaitForLBActive(currentBuilder.GetID()); err != nil {
					logger.Error("Failed to wait for loadbalancer active: ", err)
					return ctrl.Result{}, err
				}
			}
		}
	}

	// delete redundant pools, should check if pool is used by other listeners or policy then ignore
	for _, pool := range oldBuilder.GetPoolBuilders() {
		if currentPool := currentBuilder.GetPoolBuilderByName(pool.GetName()); currentPool != nil &&
			loadBalancerBuilder.GetPoolBuilderByName(pool.GetName()) == nil {
			if currentPool.IsDeleted() {
				continue
			}
			if currentBuilder.IsPoolInUseByOtherListener(currentPool.GetID()) {
				logger.Infof("pool \"%s\" is used by other listeners, ignore delete.", pool.GetName())
				continue
			}
			if err := r.Provider.DeletePool(currentBuilder.GetID(), pool.GetID()); err != nil {
				logger.Error("Failed to delete pool: ", err)
				return ctrl.Result{}, err
			}
			currentPool.SetIsDeleted(true)
			if _, err := r.Provider.WaitForLBActive(currentBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
		}
	}

	r.cacheLoadBalancerBuilder[genKey(obj.Namespace, obj.Name)] = loadBalancerBuilder
	r.resourceDependant.Set(obj, true)
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) deleteObject(ctx context.Context, obj *networkingv1.Ingress) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()
	r.resourceDependant.Clear(obj)

	if !k8s.HasFinalizer(obj, consts.IngressFinalizer) {
		return ctrl.Result{}, nil
	}

	logger.Info("Delete Object ", genKey(obj.Namespace, obj.Name))

	_, err := r.subDeleteObject(ctx, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.FinalizerManager.RemoveFinalizers(ctx, obj, consts.IngressFinalizer); err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) subDeleteObject(ctx context.Context, obj *networkingv1.Ingress) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()

	var oldBuilder builder.LoadbalancerBuilder
	var ok bool
	if oldBuilder, ok = r.cacheLoadBalancerBuilder[genKey(obj.Namespace, obj.Name)]; !ok {
		// build again
		var err error
		oldBuilder, err = builder.NewLoadBalancerBuilderByIngress(ctx, obj, r.annotationParser, r.Client,
			r.netwotkID, r.subnetID, r.subnetCIDR,
			r.Config.Cluster.ClusterID,
			r.knownNodes,
		)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// ignore reconcile
	if oldBuilder.IsIgnored() {
		logger.Info("Ingress is ignored")
		return ctrl.Result{}, nil
	}

	if oldBuilder.GetID() == "" {
		logger.Info("LoadBalancer ID is empty, return.")
		return ctrl.Result{}, nil
	}

	// inspect current loadbalancer in portal to compare with
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, oldBuilder.GetID(), r.Provider)
	if err != nil {
		logger.Error("Failed to get current loadbalancer: ", err)
		return ctrl.Result{}, err
	}

	// check if can delete whole loadbalancer
	// oldBuilder and currentBuilder should be the same listeners' name, pool's name
	checkCanDeleteWholeLoadBalancer := func(oldBuilder, currentBuilder builder.LoadbalancerBuilder) bool {
		if len(oldBuilder.GetListenerBuilders()) < len(currentBuilder.GetListenerBuilders()) {
			return false
		}
		if len(oldBuilder.GetPoolBuilders()) < len(currentBuilder.GetPoolBuilders()) {
			return false
		}
		// if listener not exists, return false
		for _, listener := range oldBuilder.GetListenerBuilders() {
			currentListener := currentBuilder.GetListenerBuilderByName(listener.GetName())
			if currentListener == nil {
				return false
			}
			// if policy not exists, return false
			currentPolicies := currentListener.GetPolicyBuilders()
			oldPolicies := listener.GetPolicyBuilders()
			if len(currentPolicies) > len(oldPolicies) {
				return false
			}
			for _, policy := range oldPolicies {
				if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy == nil {
					return false
				}
			}
		}
		// if pool not exists, return false
		for _, pool := range oldBuilder.GetPoolBuilders() {
			if currentPool := currentBuilder.GetPoolBuilderByName(pool.GetName()); currentPool == nil {
				return false
			}
		}
		return true
	}

	// if can delete whole loadbalancer, delete loadbalancer and return
	if checkCanDeleteWholeLoadBalancer(oldBuilder, currentBuilder) {
		if err := r.Provider.DeleteLoadBalancer(oldBuilder.GetID()); err != nil {
			logger.Error("Failed to delete loadbalancer: ", err)
			return ctrl.Result{}, err
		}
		logger.Infof("Delete loadbalancer \"%s\" successfully", oldBuilder.GetName())
		return ctrl.Result{}, nil
	}

	// delete redundant listeners
	for _, oldListener := range oldBuilder.GetListenerBuilders() {
		// check if can delete whole listener or just delete redundant policy
		checkCanDeleteWholeListener := func(oldListener, currentListener *builder.ListenerBuilderType) bool {
			if len(oldListener.GetPolicyBuilders()) < len(currentListener.GetPolicyBuilders()) {
				return false
			}
			for _, currentPolicy := range currentListener.GetPolicyBuilders() {
				if oldPolicy := oldListener.GetPolicyBuilderByName(currentPolicy.GetName()); oldPolicy == nil {
					return false
				}
			}
			return true
		}

		// delete whole listener
		currentListener := currentBuilder.GetListenerBuilderByName(oldListener.GetName())
		if checkCanDeleteWholeListener(oldListener, currentListener) {
			if err := r.Provider.DeleteListener(oldBuilder.GetID(), oldListener.GetID()); err != nil {
				logger.Error("Failed to delete listener: ", err)
				return ctrl.Result{}, err
			}
			if _, err := r.Provider.WaitForLBActive(oldBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
			currentListener.SetIsDeleted(true)
		} else {
			// delete redundant policy
			for _, policy := range oldListener.GetPolicyBuilders() {
				if currentPolicy := currentListener.GetPolicyBuilderByName(policy.GetName()); currentPolicy != nil {
					if err := r.Provider.DeletePolicy(currentBuilder.GetID(), currentListener.GetID(), currentPolicy.GetID()); err != nil {
						logger.Error("Failed to delete policy: ", err)
						return ctrl.Result{}, err
					}
					currentPolicy.SetIsDeleted(true)
					if _, err := r.Provider.WaitForLBActive(currentBuilder.GetID()); err != nil {
						logger.Error("Failed to wait for loadbalancer active: ", err)
						return ctrl.Result{}, err
					}
				}
			}
		}
	}

	// delete redundant pools, should check if pool is used by other listeners or policy then ignore
	for _, pool := range oldBuilder.GetPoolBuilders() {
		if currentPool := currentBuilder.GetPoolBuilderByName(pool.GetName()); currentPool != nil {
			if currentPool.IsDeleted() {
				continue
			}
			if currentBuilder.IsPoolInUseByOtherListener(currentPool.GetID()) {
				logger.Infof("pool \"%s\" is used by other listeners, ignore delete.", pool.GetName())
				continue
			}
			if err := r.Provider.DeletePool(currentBuilder.GetID(), pool.GetID()); err != nil {
				logger.Error("Failed to delete pool: ", err)
				return ctrl.Result{}, err
			}
			currentPool.SetIsDeleted(true)
			if _, err := r.Provider.WaitForLBActive(currentBuilder.GetID()); err != nil {
				logger.Error("Failed to wait for loadbalancer active: ", err)
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) init() error {
	r.cacheLoadBalancerBuilder = make(map[string]builder.LoadbalancerBuilder)

	// init other components
	r.eventClassification = event_classification.NewEventClassification(r.getObjectByKey, r.isValid)
	r.annotationParser = annotations.NewSuffixAnnotationParser(consts.INGRESS_ANNOTATION_PREFIX)
	r.resourceDependant = NewIngressDependant(r.Client)

	periodicReconciler := periodicreconciler.NewPeriodicReconciler(r, 60*time.Second, r.getReconcileRequestsPeriodically)
	ctx := context.Background()
	periodicReconciler.Start(ctx)

	return nil
}

// this function is called by the InitRunnable after cache is started, run after init() function
func (r *IngressReconciler) Init(client client.Client) error {
	r.initLock.Lock()
	defer r.initLock.Unlock()
	// should have at least 1 node to get network information (networkID, subnetID, subnetCIDR)
	var err error
	r.knownNodes, err = r.getNodes(client)
	if err != nil {
		return err
	}
	providerIDs := builder.VNGHelper.GetListProviderID(r.knownNodes)
	if len(r.knownNodes) == 0 || len(providerIDs) == 0 {
		return errs.ErrorNoNodeAtInitTime
	}

	// init provider
	err = r.Provider.Init(providerIDs)
	if err != nil {
		logrus.Error("Failed to init provider: ", err)
		return err
	}
	r.netwotkID = r.Provider.GetNetworkID()
	r.subnetID = r.Provider.GetSubnetID()
	r.subnetCIDR = r.Provider.GetSubnetCIDR()
	if r.netwotkID == "" || r.subnetID == "" || r.subnetCIDR == "" {
		return errs.ErrorNoNetworkInfo
	}

	r.initialized = true
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := r.init()
	if err != nil {
		return err
	}

	// Add the initialization logic after cache is started
	if err := mgr.Add(&InitRunnable{
		Client:     mgr.GetClient(),
		Reconciler: r, // Pass the reconciler to store nodes
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Watches(
			&corev1.Endpoints{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, endpoint client.Object) []reconcile.Request {
				return r.resourceDependant.GetResourceNeedReconcile("endpoint", endpoint.GetNamespace(), endpoint.GetName())
			}),
			k8sBuilder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, service client.Object) []reconcile.Request {
				return r.resourceDependant.GetResourceNeedReconcile("service", service.GetNamespace(), service.GetName())
			}),
			k8sBuilder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
				// list all
				objList := &networkingv1.IngressList{}
				err := r.List(ctx, objList)
				if err != nil {
					return []reconcile.Request{}
				}

				// filter
				requests := make([]reconcile.Request, 0)
				for _, obj := range objList.Items {
					if obj.Spec.IngressClassName != nil && *obj.Spec.IngressClassName == consts.IngressClass {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      obj.GetName(),
								Namespace: obj.GetNamespace(),
							},
						})
					}
				}
				return requests
			}),
			k8sBuilder.WithPredicates(predicate.AnnotationChangedPredicate{}),
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
					logrus.Info("Detect create Ingress event.")
					return true
				case *corev1.Service:
					return false
				case *corev1.Endpoints:
					return false
				case *corev1.Node:
					newNodes, err := r.getNodes(r.Client)
					if err != nil {
						logrus.Warn("Detect create Node event but failed to get nodes: ", err)
						return true
					}
					if !k8s.NodeSlicesEqual(r.knownNodes, newNodes) {
						logrus.Info("Detect create Node event, update knownNodes.")
						r.knownNodes = newNodes
						return true
					}
					return false
				default:
					logrus.Warn("Detect create object is of an unknown type: ", e.Object)
					return false
				}
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				switch e.ObjectNew.(type) {
				case *networkingv1.Ingress:
					oldObj := e.ObjectOld.(*networkingv1.Ingress)
					newObj := e.ObjectNew.(*networkingv1.Ingress)

					if (oldObj.Spec.IngressClassName == nil || *oldObj.Spec.IngressClassName != consts.IngressClass) &&
						(newObj.Spec.IngressClassName == nil || *newObj.Spec.IngressClassName != consts.IngressClass) {
						return false
					}

					if !reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
						logrus.Info("Detect update Ingress Spec event.")
						return true
					}

					// remove whitelisted annotations in the comparison
					for k := range consts.WhitelistedAnnotations {
						delete(oldObj.Annotations, k)
						delete(newObj.Annotations, k)
					}
					if !reflect.DeepEqual(oldObj.Annotations, newObj.Annotations) {
						logrus.Info("Detect update Ingress Annotations event.")
						return true
					}
					if !reflect.DeepEqual(oldObj.DeletionTimestamp.IsZero(), newObj.DeletionTimestamp.IsZero()) {
						logrus.Info("Detect update Ingress DeletionTimestamp event.")
						return true
					}
					return false

				case *corev1.Service:
					logrus.Info("Detect update Service event.")
					return true

				case *corev1.Endpoints:
					logrus.Info("Detect update Endpoints event.")
					return true
				case *corev1.Node:
					oldObj := e.ObjectOld.(*corev1.Node)
					newObj := e.ObjectNew.(*corev1.Node)
					if oldObj.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] != newObj.Annotations[consts.LABEL_NODE_EXCLUDE_LOADBALANCER] {
						logrus.Info("Detect update Node Annotations event.")
						r.knownNodes, _ = r.getNodes(r.Client)
						return true
					}
					return false
				default:
					logrus.Warn("Detect update object is of an unknown type: ", e.ObjectNew)
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
					logrus.Info("Detect delete Ingress event.")
					return true
				case *corev1.Service:
					logrus.Info("Detect delete Service event.")
					return true
				case *corev1.Endpoints:
					logrus.Info("Detect delete Endpoints event.")
					return true
				case *corev1.Node:
					newNodes, err := r.getNodes(r.Client)
					if err != nil {
						logrus.Warn("Detect delete Node event but failed to get nodes: ", err)
						return true
					}
					if !k8s.NodeSlicesEqual(r.knownNodes, newNodes) {
						logrus.Info("Detect delete Node event, update knownNodes.")
						r.knownNodes = newNodes
						return true
					}
					return false
				default:
					logrus.Warn("Detect delete object is of an unknown type: ", e.Object)
					return false
				}
			},
		}).
		Complete(r)
}
