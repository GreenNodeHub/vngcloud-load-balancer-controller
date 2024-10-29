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
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/huandu/go-clone"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	Config           *config.Config
	FinalizerManager k8s.FinalizerManager

	eventClassification *event_classification.EventClassification
	annotationParser    annotations.Parser
	resourceDependant   ResourceDependant[*corev1.Service]

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

	updateTracker       *UpdateTracker
	timeReconcilePeriod time.Duration
	numCurrentReconcile int
	numCurrentLock      sync.Mutex
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=nodes/finalizers,verbs=update

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update

// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=endpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=endpoints/finalizers,verbs=update

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups="",resources=node,verbs=get;list;watch
func (r *ServiceReconciler) isValid(obj client.Object) bool {
	object, ok := obj.(*corev1.Service)
	if !ok {
		return false
	}
	if object.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	return true
}

func (r *ServiceReconciler) getNodes(client client.Client) ([]*corev1.Node, error) {
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

func (r *ServiceReconciler) getObjectByKey(key string) (client.Object, bool) {
	namespace, name := revertKey(key)
	resource := &corev1.Service{}
	err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, resource)
	if err != nil {
		return nil, false
	}
	return resource, true
}

func (r *ServiceReconciler) updateObjectAnnotation(ctx context.Context, obj client.Object, annos map[string]string) error {
	if obj == nil {
		return nil
	}
	logger := contexts.NewContext(ctx).Log()

	// get object again to avoid conflict
	err := r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
	if err != nil {
		logger.Error("Failed to get object: ", err)
		return err
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	logger.Debugf("Update annotation for object %s/%s", obj.GetNamespace(), obj.GetName())
	newAnnotation := clone.Clone(annotations).(map[string]string)
	for k, v := range annos {
		newAnnotation[k] = v
	}
	debugCompareMapString(annotations, newAnnotation)

	for k, v := range annos {
		annotations[k] = v
	}
	obj.SetAnnotations(annotations)
	return r.Update(ctx, obj, &client.UpdateOptions{})
}

func (r *ServiceReconciler) updateObjectStatus(ctx context.Context, obj client.Object, address string) error {
	if obj == nil {
		return nil
	}
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Update status for object %s/%s = %s", obj.GetNamespace(), obj.GetName(), address)

	// get object again to avoid conflict
	err := r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
	if err != nil {
		logger.Error("Failed to get object: ", err)
		return err
	}

	// update status
	if address == "" {
		return errors.New("address is empty")
	}

	ingress := obj.(*corev1.Service)

	addr := net.ParseIP(address)
	if addr != nil {
		ingress.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: address}}
	} else {
		ingress.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: address}}
	}
	return r.Status().Update(ctx, obj)
}

func (r *ServiceReconciler) startBackgroundGoroutine(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.timeReconcilePeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if r.numCurrentReconcile > 0 {
					logrus.Debug("Skip reconcile periodically.")
				} else {
					// get all loadbalancers
					loadBalancers, err := r.Provider.ListLoadBalancers(context.Background())
					if err != nil {
						logrus.Error("Failed to list loadbalancers: ", err)
						continue
					}

					// get all resources need to reconcile
					requests := r.updateTracker.GetReconcileRequests(loadBalancers)
					for _, req := range requests {
						r.Enqueue(req)
					}
				}

			case <-ctx.Done():
				// Exit the Goroutine when the context is cancelled (e.g., operator shutdown)
				return
			}
		}
	}()
}

// Enqueue will trigger the reconcile manually
func (r *ServiceReconciler) Enqueue(req reconcile.Request) {
	key := genKey(req.Namespace, req.Name)
	obj, exists := r.getObjectByKey(key)
	if !exists {
		return
	}
	err := r.updateObjectAnnotation(context.Background(), obj,
		map[string]string{fmt.Sprintf("%s/%s", r.annotationParser.GetPrefix(), "trigger"): fmt.Sprintf("%d", time.Now().Unix())})
	if err != nil {
		logrus.Error("Failed to update annotation: ", err)
	}
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
	if !r.initialized {
		return ctrl.Result{Requeue: true}, nil
	}

	// add one more reconcile to the counter
	r.numCurrentLock.Lock()
	r.numCurrentReconcile++
	r.numCurrentLock.Unlock()
	defer func() {
		r.numCurrentLock.Lock()
		r.numCurrentReconcile--
		r.numCurrentLock.Unlock()
	}()

	ctx = contexts.NewContext(ctx).SetLogName(fmt.Sprint("s/" + genKey(req.Namespace, req.Name))).GetContext()
	logger := contexts.NewContext(ctx).Log()
	logger.Info("------------------ START ------------------")
	defer logger.Info("------------------ DONE ------------------")

	err := r.reconcile(ctx, req)

	// some errors should not requeue
	if err != nil {
		switch {
		case errs.IsExceededSecurityGroupPerServerQuota(err),
			errs.IsLoadBalancerNotFound(err):
			err = errs.NewNoNeedRequeue(err.Error())
		}
	}

	return errs.HandleReconcileError(err, logger)
}

func (r *ServiceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
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

	// for testing
	if r.modeTest {
		switch event.Type {
		case event_classification.DeleteEvent:
			r.resourceDependant.Clear(event.Obj.(*corev1.Service))
			_, err := r.deleteTest(ctx, req)
			return err
		default:
			r.resourceDependant.Set(event.Obj.(*corev1.Service), true)
			_, err := r.ensureTest(ctx, req)
			return err
		}
	}

	switch event.Type {
	case event_classification.DeleteEvent:
		obj := event.Obj.(*corev1.Service)
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
		obj := event.Obj.(*corev1.Service)
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
		obj := event.Obj.(*corev1.Service)
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

func (r *ServiceReconciler) ensureObject(ctx context.Context, obj *corev1.Service, oldObjInterface interface{}) error {
	logger := contexts.NewContext(ctx).Log()

	if err := r.FinalizerManager.AddFinalizers(ctx, obj, consts.ServiceFinalizer); err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return err
	}

	loadBalancerBuilder, err := builder.NewModelBuilderByService(ctx, obj, r.annotationParser, r.Client,
		r.netwotkID, r.subnetID, r.subnetCIDR,
		r.Config.Cluster.ClusterID,
		r.knownNodes,
		r.cniMode,
		r.defaultPackageID,
	)
	if err != nil {
		logger.Error("Failed to create loadbalancer builder: ", err)
		return err
	}

	// logger.Info("ModelBuilder: ", loadBalancerBuilder.StringFormat(), " ", loadBalancerBuilder.JSONFormat())
	// loadBalancerBuilder.Print()

	// ignore reconcile
	if loadBalancerBuilder.IsIgnored() {
		logger.Info("Object is ignored")
		return nil
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
			return err
		}
		if lb == nil {
			// create loadbalancer. It mays create lb, listener, pool at the same time
			lb, err = r.Provider.CreateLoadBalancer(ctx, loadBalancerBuilder.CreateLoadBalancerOptions())
			if err != nil {
				return err
			}
		}
		if lb == nil || lb.UUID == "" {
			return errors.New("load balancer not have UUID after find by name or create, need to retry")
		}

		// wait for loadbalancer active, if lb is error, delete it and return error
		if _, err := r.Provider.WaitForLBActive(ctx, lb.UUID); err != nil {
			if err == provider.ErrorLoadBalancerStatusError {
				if err := r.Provider.DeleteLoadBalancer(ctx, lb.UUID); err != nil {
					logger.Error("Failed to delete loadbalancer: ", err)
					return err
				}
				logger.Infof("Delete loadbalancer \"%s\" because of status error, recreate now.", lb.UUID)
				return errs.NewRequeueNeeded("loadbalancer status is error, delete and recreate")
			}
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}

		// update object annotation, also trigger reconcile immediately
		r.updateObjectAnnotation(ctx, obj, map[string]string{
			fmt.Sprintf("%s/%s", consts.INGRESS_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID): lb.UUID,
		})
		return nil
	} else {
		if _, err := r.Provider.WaitForLBActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}

	// inspect current loadbalancer in portal to compare with the new one
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, loadBalancerBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser, r.Config.Cluster.ClusterID, r.knownNodes, obj)
	if err != nil {
		logger.Error("Failed to get current loadbalancer: ", err)
		return err
	}

	// get old object annotations
	oldAnnotations := make(map[string]string)
	if oldObj, ok := oldObjInterface.(*corev1.Service); ok {
		oldAnnotations = oldObj.Annotations
	}

	// build oldBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, oldAnnotations, r.annotationParser)

	// ensure tags
	err = currentBuilder.EnsureTags(loadBalancerBuilder.GetTags(), oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure tags: ", err)
		return err
	}

	// ensure package
	if currentBuilder.GetPackageID() != loadBalancerBuilder.GetPackageID() &&
		currentBuilder.GetPackageID() != "" &&
		loadBalancerBuilder.GetPackageID() != "" {
		if err := r.Provider.ResizeLoadBalancer(ctx, loadBalancerBuilder.GetLoadBalancerID(), loadBalancerBuilder.GetPackageID()); err != nil {
			logger.Error("Failed to resize loadbalancer: ", err)
			return err
		}
		if _, err := r.Provider.WaitForLBActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}

	// ensure pools
	for _, poolBuilder := range loadBalancerBuilder.GetPoolBuilders() {
		err := currentBuilder.EnsurePool(poolBuilder, oldBuilder)
		if err != nil {
			logger.Error("Failed to ensure pool: ", err)
			return err
		}
	}

	// ensure listeners
	for _, listenerBuilder := range loadBalancerBuilder.GetListenerBuilders() {
		// set default pool id created above
		referPool := loadBalancerBuilder.GetPoolBuilderByName(listenerBuilder.GetPoolName())
		if referPool == nil {
			logger.Error("Failed to get refer pool: ", listenerBuilder.GetPoolName())
			return nil
		}
		listenerBuilder.SetPoolID(referPool.GetID())

		err := currentBuilder.EnsureListener(listenerBuilder, oldBuilder)
		if err != nil {
			logger.Error("Failed to ensure listener: ", err)
			return err
		}
	}

	// delete redundant listeners
	err = currentBuilder.DeleteRedundantListeners(oldBuilder, loadBalancerBuilder)
	if err != nil {
		logger.Error("Failed to delete redundant listener: ", err)
		return err
	}

	// delete redundant pools, should check if pool is used by other listeners then ignore
	err = currentBuilder.DeleteRedundantPools(oldBuilder, loadBalancerBuilder)
	if err != nil {
		logger.Error("Failed to delete redundant pool: ", err)
		return err
	}

	// update management annotations
	if err := r.updateObjectAnnotation(ctx, obj, loadBalancerBuilder.GetManageAnnotation()); err != nil {
		logger.Error("Failed to update management annotations: ", err)
		return err
	}

	// watch endpoint if target type is ip or cni mode is cilium native routing
	r.resourceDependant.Set(obj, loadBalancerBuilder.GetTargetType() == builder.TargetTypeIP ||
		r.cniMode == utils.CiliumNativeRouting)

	// get lb updated time and add to update tracker
	lb, err := r.Provider.GetLoadBalancerByID(ctx, loadBalancerBuilder.GetLoadBalancerID())
	if err != nil {
		logger.Error("Failed to get loadbalancer: ", err)
		return err
	}
	r.updateTracker.AddUpdateTracker(loadBalancerBuilder.GetLoadBalancerID(), obj.GetNamespace(), obj.GetName(), lb.UpdatedAt)

	// update status
	err = r.updateObjectStatus(ctx, obj, lb.Address)
	if err != nil {
		logger.Error("Failed to update status: ", err)
		return err
	}

	// ensure security group with mutex
	err = r.ensureSecurityGroup(currentBuilder, loadBalancerBuilder, oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure security group: ", err)
		return err
	}

	return nil
}

func (r *ServiceReconciler) ensureSecurityGroup(currentBuilder builder.LoadBalancerBuilder, loadBalancerBuilder builder.ModelBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureSecurityGroups(loadBalancerBuilder, oldBuilder)
	if err != nil {
		return err
	}
	return nil
}

func (r *ServiceReconciler) deleteObject(ctx context.Context, obj *corev1.Service) error {
	logger := contexts.NewContext(ctx).Log()
	r.resourceDependant.Clear(obj)

	if !k8s.HasFinalizer(obj, consts.ServiceFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	err := r.subDeleteObject(ctx, obj)
	if err != nil {
		return err
	}

	if err := r.FinalizerManager.RemoveFinalizers(ctx, obj, consts.ServiceFinalizer); err != nil {
		// r.eventRecorder.Event(obj, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedRemoveFinalizer, fmt.Sprintf("Failed remove finalizer due to %v", err))
		return err
	}
	return nil
}

func (r *ServiceReconciler) subDeleteObject(ctx context.Context, obj *corev1.Service) error {
	logger := contexts.NewContext(ctx).Log()

	// build oldBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, obj.Annotations, r.annotationParser)

	// ignore reconcile
	if oldBuilder.IsIgnored() {
		logger.Info("Service is ignored")
		return nil
	}

	if oldBuilder.GetLoadBalancerID() == "" {
		logger.Info("LoadBalancer ID is empty, return.")
		return nil
	}

	// remove from update tracker
	r.updateTracker.RemoveUpdateTracker(oldBuilder.GetLoadBalancerID(), obj.GetNamespace(), obj.GetName())

	// inspect current loadbalancer in portal to compare with
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, oldBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser, r.Config.Cluster.ClusterID, r.knownNodes, obj)
	if err != nil {
		if errs.IsLoadBalancerNotFound(err) {
			logger.Info("LoadBalancer not found, return.")
			return nil
		}
		logger.Error("Failed to get current loadbalancer: ", err)
		return err
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
		return err
	}

	// check if can delete whole loadbalancer
	// oldBuilder and currentBuilder should be the same listeners' name, pool's name
	// if can delete whole loadbalancer, delete loadbalancer and return
	if currentBuilder.CanDeleteWholeLoadBalancer(oldBuilder) {
		if err := r.Provider.DeleteLoadBalancer(ctx, oldBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to delete loadbalancer: ", err)
			return err
		}
		logger.Infof("Delete loadbalancer \"%s\" successfully", oldBuilder.GetLoadBalancerID())
	} else {
		// delete redundant listeners
		err = currentBuilder.DeleteRedundantListeners(oldBuilder, newBuilder)
		if err != nil {
			logger.Error("Failed to delete redundant listeners: ", err)
			return err
		}

		// delete redundant pools, should check if pool is used by other listeners or policy then ignore
		err = currentBuilder.DeleteRedundantPools(oldBuilder, newBuilder)
		if err != nil {
			logger.Error("Failed to delete redundant pools: ", err)
			return err
		}
	}

	// ensure delete security group with mutex
	err = r.ensureDeleteSecurityGroup(currentBuilder, oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure delete security group: ", err)
		return err
	}

	return nil
}

func (r *ServiceReconciler) ensureDeleteSecurityGroup(currentBuilder builder.LoadBalancerBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureDeleteSecurityGroups(oldBuilder)
	if err != nil {
		return err
	}
	return nil
}

func (r *ServiceReconciler) init() error {
	// init other components
	r.eventClassification = event_classification.NewEventClassification(r.getObjectByKey, r.isValid)
	r.annotationParser = annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
	r.resourceDependant = NewServiceDependant(r.Client)
	r.updateTracker = NewUpdateTracker()
	if r.timeReconcilePeriod == 0 {
		r.timeReconcilePeriod = 60 * time.Second
	}

	ctx := context.Background()
	r.startBackgroundGoroutine(ctx)

	return nil
}

// this function is called by the InitRunnable after cache is started, run after init() function
func (r *ServiceReconciler) Init(client client.Client) error {
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
		return errors.New("require at least 1 node to get network information")
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
		return errors.New("no network info, lack of networkID or subnetID or subnetCIDR")
	}

	// if clusterID is empty, get from node label
	if r.Config.Cluster.ClusterID == "" {
		clusterID := ""
		for _, node := range r.knownNodes {
			if node.Labels != nil && node.Labels["vks.vngcloud.vn/cluster-id"] != "" {
				clusterID = node.Labels["vks.vngcloud.vn/cluster-id"]
				break
			}
		}
		if clusterID == "" {
			return errors.New("no clusterID found, should exist in node label or specify in config")
		}
		r.Config.Cluster.ClusterID = clusterID
		logrus.Infof("ClusterID is empty, get from node label: %s", r.Config.Cluster.ClusterID)
	}

	// init cni mode
	r.cniMode, err = utils.NewDetector(client).DetectCNIType()
	if err != nil {
		logrus.Error("Failed to detect CNI type: ", err)
		return err
	}
	logrus.Infof("Detected CNI type: %s", r.cniMode)

	// get default package id
	r.defaultPackageID, _, err = r.Provider.GetDefaultPackage()
	if err != nil {
		logrus.Error("Failed to get default package: ", err)
		return err
	}

	r.initialized = true
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		For(&corev1.Service{}).
		Watches(
			&corev1.Endpoints{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, endpoint client.Object) []reconcile.Request {
				return r.resourceDependant.GetResourceNeedReconcile("endpoint", endpoint.GetNamespace(), endpoint.GetName())
			}),
			k8sBuilder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
				// list all
				objList := &corev1.ServiceList{}
				err := r.List(ctx, objList)
				if err != nil {
					return []reconcile.Request{}
				}

				// filter
				requests := make([]reconcile.Request, 0)
				for _, obj := range objList.Items {
					if obj.Spec.Type == corev1.ServiceTypeLoadBalancer {
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
				case *corev1.Service:
					if object := e.Object.(*corev1.Service); object.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}
					logrus.Info("Detect create Service event.")
					return true
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
				case *corev1.Service:
					oldObj := e.ObjectOld.(*corev1.Service)
					newObj := e.ObjectNew.(*corev1.Service)

					if oldObj.Spec.Type != corev1.ServiceTypeLoadBalancer && newObj.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}

					if !reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
						logrus.Info("Detect update Service Spec event.")
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
						logrus.Info("Detect update Service Annotations event.")
						debugCompareMapString(oldAnnotations, newAnnotations)
						return true
					}
					if !reflect.DeepEqual(oldObj.DeletionTimestamp.IsZero(), newObj.DeletionTimestamp.IsZero()) {
						logrus.Info("Detect update Service DeletionTimestamp event.")
						return true
					}
					return false

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
				// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
				// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object. Since
				// deletion is already taken care of during update event, we will ignore this event.
				case *corev1.Service:
					if object := e.Object.(*corev1.Service); object.Spec.Type != corev1.ServiceTypeLoadBalancer {
						return false
					}
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
