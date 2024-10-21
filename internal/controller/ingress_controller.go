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
	"sync"
	"time"

	"github.com/huandu/go-clone"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
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

	//  flag to check if the reconciler is initialized
	initialized bool
	initLock    sync.Mutex

	cniMode          utils.CNIType
	defaultPackageID string
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

func (r *IngressReconciler) updateObjectAnnotation(ctx context.Context, obj *networkingv1.Ingress, annos map[string]string) error {
	if obj == nil {
		return nil
	}
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Update annotation for object %s/%s: %v", obj.Namespace, obj.Name, annos)
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}
	for k, v := range annos {
		obj.Annotations[k] = v
	}
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

	result, err := r.reconcile(ctx, req)
	if err != nil {
		// handle some particular errors
		if errs.IsExceededSecurityGroupPerServerQuota(err) {
			return ctrl.Result{}, nil
		}
		if errs.IsLoadBalancerNotFound(err) {
			return ctrl.Result{}, nil
		}

		switch err {
		// misconfiguration, ignore these errors, reconcile again when resource is updated
		case
			errs.ErrorMissingCertificates,
			errs.ErrorServicePortNotFound,
			errs.ErrorSecurityGroupNotFound,
			errs.ErrorServicePortEmpty:
			return ctrl.Result{}, nil

		// no need to reconcile again
		case
			errs.ErrorSecurityGroupInUse:
			return ctrl.Result{}, nil

		// sleep 5 seconds and requeue
		default:
			time.Sleep(5 * time.Second)
			return ctrl.Result{}, err
		}
	}
	return result, nil
}

func (r *IngressReconciler) reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := genKey(req.Namespace, req.Name)

	ctx = contexts.NewContext(ctx).SetLogName(fmt.Sprint("i/" + key)).GetContext()
	logger := contexts.NewContext(ctx).Log()
	defer logger.Info("------------------ DONE ------------------")

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
		obj := event.Obj.(*networkingv1.Ingress)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Deleting", key)
		result, err := r.deleteObject(ctx, obj)
		if err == nil {
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Deleted", key)
		} else {
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedDelete", err.Error())
		}
		return result, err
	case event_classification.CreateEvent:
		obj := event.Obj.(*networkingv1.Ingress)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Creating", key)
		result, err := r.ensureObject(ctx, obj, nil)
		if err == nil {
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Created", key)
		} else {
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedCreate", err.Error())
		}
		return result, err
	default:
		obj := event.Obj.(*networkingv1.Ingress)
		r.Recorder.Event(obj, corev1.EventTypeNormal, "Updating", key)
		result, err := r.ensureObject(ctx, obj, event.OldObj)
		if err == nil {
			r.Recorder.Event(obj, corev1.EventTypeNormal, "Updated", key)
		} else {
			r.Recorder.Event(obj, corev1.EventTypeWarning, "FailedUpdate", err.Error())
		}
		return result, err
	}
}

func (r *IngressReconciler) ensureObject(ctx context.Context, obj *networkingv1.Ingress, oldObjInterface interface{}) (ctrl.Result, error) {
	logger := contexts.NewContext(ctx).Log()

	if err := r.FinalizerManager.AddFinalizers(ctx, obj, consts.IngressFinalizer); err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrl.Result{}, err
	}

	logger.Info("Reconcile Object ", genKey(obj.Namespace, obj.Name))

	loadBalancerBuilder, err := builder.NewModelBuilderByIngress(ctx, obj, r.annotationParser, r.Client,
		r.netwotkID, r.subnetID, r.subnetCIDR,
		r.Config.Cluster.ClusterID,
		r.knownNodes,
		r.cniMode,
		r.defaultPackageID,
	)
	if err != nil {
		logger.Error("Failed to create loadbalancer builder: ", err)
		return ctrl.Result{}, err
	}

	// logger.Info("ModelBuilder: ", loadBalancerBuilder.StringFormat(), " ", loadBalancerBuilder.JSONFormat())
	// loadBalancerBuilder.Print()

	// ignore reconcile
	if loadBalancerBuilder.IsIgnored() {
		logger.Info("Object is ignored")
		return ctrl.Result{}, nil
	}

	// create loadbalancer, update annotation and reconcile later
	if loadBalancerBuilder.GetLoadBalancerID() == "" {
		// check if loadbalancer with the generate name exists, if exists, update annotation and return
		// check the name that user specified in the annotation first
		lbName := loadBalancerBuilder.GetLoadBalancerName()
		if lbName == "" {
			lbName = loadBalancerBuilder.GetLoadBalancerDefaultName()
		}
		lb, err := r.Provider.GetLoadBalancerByName(ctx, lbName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if lb == nil {
			// create loadbalancer. It mays create lb, listener, pool at the same time
			lb, err = r.Provider.CreateLoadBalancer(ctx, loadBalancerBuilder.CreateLoadBalancerOptions())
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		if lb == nil || lb.UUID == "" {
			return ctrl.Result{}, errs.ErrorLoadBalancerNotHaveUUID
		}
		r.updateObjectAnnotation(ctx, obj, map[string]string{
			fmt.Sprintf("%s/%s", consts.INGRESS_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID): lb.UUID,
		})
		return ctrl.Result{}, nil
	} else {
		if _, err := r.Provider.WaitForLBActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return ctrl.Result{}, err
		}
	}

	// inspect current loadbalancer in portal to compare with the new one
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, loadBalancerBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser, r.Config.Cluster.ClusterID, r.knownNodes, obj)
	if err != nil {
		logger.Error("Failed to get current loadbalancer: ", err)
		return ctrl.Result{}, err
	}

	// get old object annotations
	oldAnnotations := make(map[string]string)
	if oldObj, ok := oldObjInterface.(*networkingv1.Ingress); ok {
		oldAnnotations = oldObj.Annotations
	}

	// build oldBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, oldAnnotations, r.annotationParser)

	// ensure tags
	err = currentBuilder.EnsureTags(loadBalancerBuilder.GetTags(), oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure tags: ", err)
		return ctrl.Result{}, err
	}

	// ensure package
	if currentBuilder.GetPackageID() != loadBalancerBuilder.GetPackageID() &&
		currentBuilder.GetPackageID() != "" &&
		loadBalancerBuilder.GetPackageID() != "" {
		if err := r.Provider.ResizeLoadBalancer(ctx, loadBalancerBuilder.GetLoadBalancerID(), loadBalancerBuilder.GetPackageID()); err != nil {
			logger.Error("Failed to resize loadbalancer: ", err)
			return ctrl.Result{}, err
		}
		if _, err := r.Provider.WaitForLBActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return ctrl.Result{}, err
		}
	}

	// ensure pools
	for _, poolBuilder := range loadBalancerBuilder.GetPoolBuilders() {
		err := currentBuilder.EnsurePool(poolBuilder, oldBuilder)
		if err != nil {
			logger.Error("Failed to ensure pool: ", err)
			return ctrl.Result{}, err
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

		err := currentBuilder.EnsureListener(listenerBuilder, oldBuilder)
		if err != nil {
			logger.Error("Failed to ensure listener: ", err)
			return ctrl.Result{}, err
		}
	}

	// delete redundant listeners
	err = currentBuilder.DeleteRedundantListeners(oldBuilder, loadBalancerBuilder)
	if err != nil {
		logger.Error("Failed to delete redundant listener: ", err)
		return ctrl.Result{}, err
	}

	// delete redundant pools, should check if pool is used by other listeners or policy then ignore
	err = currentBuilder.DeleteRedundantPools(oldBuilder, loadBalancerBuilder)
	if err != nil {
		logger.Error("Failed to delete redundant pool: ", err)
		return ctrl.Result{}, err
	}

	// ensure security group with mutex
	err = r.ensureSecurityGroup(currentBuilder, loadBalancerBuilder, oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure security group: ", err)
		return ctrl.Result{}, err
	}

	// update management annotations
	if err := r.updateObjectAnnotation(ctx, obj, loadBalancerBuilder.GetManageAnnotation()); err != nil {
		logger.Error("Failed to update management annotations: ", err)
		return ctrl.Result{}, err
	}

	// watch endpoint if target type is ip or cni mode is cilium native routing
	r.resourceDependant.Set(obj, loadBalancerBuilder.GetTargetType() == builder.TargetTypeIP ||
		r.cniMode == utils.CiliumNativeRouting)
	return ctrl.Result{}, nil
}

func (r *IngressReconciler) ensureSecurityGroup(currentBuilder builder.LoadBalancerBuilder, loadBalancerBuilder builder.ModelBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureSecurityGroups(loadBalancerBuilder, oldBuilder)
	if err != nil {
		return err
	}
	return nil
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

	// build oldBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, obj.Annotations, r.annotationParser)

	// ignore reconcile
	if oldBuilder.IsIgnored() {
		logger.Info("Ingress is ignored")
		return ctrl.Result{}, nil
	}

	if oldBuilder.GetLoadBalancerID() == "" {
		logger.Info("LoadBalancer ID is empty, return.")
		return ctrl.Result{}, nil
	}

	// inspect current loadbalancer in portal to compare with
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, oldBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser, r.Config.Cluster.ClusterID, r.knownNodes, obj)
	if err != nil {
		if errs.IsLoadBalancerNotFound(err) {
			logger.Info("LoadBalancer not found, return.")
			return ctrl.Result{}, nil
		}
		logger.Error("Failed to get current loadbalancer: ", err)
		return ctrl.Result{}, err
	}

	// build loadbalancer model, pass nil to object to create a model with default values
	newBuilder, err := builder.NewModelBuilderByIngress(ctx, nil, r.annotationParser, r.Client,
		r.netwotkID, r.subnetID, r.subnetCIDR,
		r.Config.Cluster.ClusterID,
		r.knownNodes,
		r.cniMode,
		r.defaultPackageID,
	)
	if err != nil {
		logger.Error("Failed to create new model builder: ", err)
		return ctrl.Result{}, err
	}

	// check if can delete whole loadbalancer
	// oldBuilder and currentBuilder should be the same listeners' name, pool's name
	// if can delete whole loadbalancer, delete loadbalancer and return
	if currentBuilder.CanDeleteWholeLoadBalancer(oldBuilder) {
		if err := r.Provider.DeleteLoadBalancer(ctx, oldBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to delete loadbalancer: ", err)
			return ctrl.Result{}, err
		}
		logger.Infof("Delete loadbalancer \"%s\" successfully", oldBuilder.GetLoadBalancerID())
	} else {
		// delete redundant listeners
		err = currentBuilder.DeleteRedundantListeners(oldBuilder, newBuilder)
		if err != nil {
			logger.Error("Failed to delete redundant listeners: ", err)
			return ctrl.Result{}, err
		}

		// delete redundant pools, should check if pool is used by other listeners or policy then ignore
		err = currentBuilder.DeleteRedundantPools(oldBuilder, newBuilder)
		if err != nil {
			logger.Error("Failed to delete redundant pools: ", err)
			return ctrl.Result{}, err
		}
	}

	// ensure delete security group with mutex
	err = r.ensureDeleteSecurityGroup(currentBuilder, oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure delete security group: ", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) ensureDeleteSecurityGroup(currentBuilder builder.LoadBalancerBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureDeleteSecurityGroups(oldBuilder)
	if err != nil {
		return err
	}
	return nil
}

func (r *IngressReconciler) init() error {
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

	// init cni mode
	r.cniMode, err = utils.NewDetector(client).DetectCNIType()
	if err != nil {
		logrus.Error("Failed to detect CNI type: ", err)
		return err
	}
	logrus.Infof("Detected CNI type: %s", r.cniMode)

	// get default package id
	_, r.defaultPackageID, err = r.Provider.GetDefaultPackage()
	if err != nil {
		logrus.Error("Failed to get default package: ", err)
		return err
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

					oldAnnotations := clone.Clone(oldObj.Annotations).(map[string]string)
					newAnnotations := clone.Clone(newObj.Annotations).(map[string]string)

					// remove whitelisted annotations in the comparison
					for k := range consts.WhitelistedAnnotations {
						delete(oldAnnotations, k)
						delete(newAnnotations, k)
					}
					if !reflect.DeepEqual(oldAnnotations, newAnnotations) {
						logrus.Info("Detect update Ingress Annotations event.")
						logrus.Debugf("Old annotations: %v", oldObj.Annotations)
						logrus.Debugf("New annotations: %v", newObj.Annotations)
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
