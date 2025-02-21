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

	"github.com/anngdinh/operator-helper/contexts"
	"github.com/anngdinh/operator-helper/event_classification"
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/huandu/go-clone"
	"github.com/pkg/errors"
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

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	builder "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/builder_glb"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
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
	annotationParser    annotations.Parser
	resourceDependant   ResourceDependant[*v1alpha1.VngcloudGlobalLoadBalancer]

	netwotkID   string
	subnetID    string
	networkCIDR string

	knownNodes       []*corev1.Node
	cniMode          utils.CNIType
	defaultPackageID string

	//  flag to check if the reconciler is initialized
	initialized bool
	initLock    sync.Mutex

	numCurrentReconcile int
	numCurrentLock      sync.Mutex
}

func (r *VngcloudGlobalLoadBalancerReconciler) init() error {
	r.eventClassification = event_classification.NewEventClassification(r.getObjectByKey, r.isValid)
	r.annotationParser = annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
	r.resourceDependant = NewVGLBDependant(r.Client)
	r.defaultPackageID = "pkg-b02e62ab-a282-4faf-8732-a172ef497a7b"
	return nil
}

// this function is called by the InitRunnable after cache is started, run after init() function
func (r *VngcloudGlobalLoadBalancerReconciler) Init(client client.Client) error {
	r.initLock.Lock()
	defer r.initLock.Unlock()
	// should have at least 1 node to get network information (networkID, subnetID, networkCIDR)
	var err error
	r.knownNodes, err = r.getNodes(client)
	if err != nil {
		return err
	}
	providerIDs := builder.GetListProviderID(r.knownNodes)
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
	r.networkCIDR = r.Provider.GetNetworkCIDR()
	if r.netwotkID == "" || r.subnetID == "" || r.networkCIDR == "" {
		return errors.New("no network info, lack of networkID or subnetID or networkCIDR")
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
		logrus.Infof("ClusterID is empty, get from node label: %s.", r.Config.Cluster.ClusterID)
	}

	// if region is empty, get from node label
	if r.Config.Cluster.Region == "" {
		region := ""
		for _, node := range r.knownNodes {
			if node.Labels != nil && node.Labels["vks.vngcloud.vn/mgmt-zone"] != "" {
				region = node.Labels["vks.vngcloud.vn/mgmt-zone"]
				break
			}
		}
		if region == "" {
			return errors.New("no region found, should exist in node label or specify in config")
		}
		if strings.HasPrefix(region, "han") {
			region = "han"
		} else {
			region = "hcm"
		}
		r.Config.Cluster.Region = region
		logrus.Infof("Region is empty, get from node label: %s.", r.Config.Cluster.Region)
	}

	// init cni mode
	r.cniMode, err = utils.NewDetector(client).DetectCNIType()
	if err != nil {
		logrus.Error("Failed to detect CNI type: ", err)
		return err
	}
	logrus.Infof("Detected CNI type: %s", r.cniMode)

	// r.UpdateTracker.Start(context.Background(), r.Config.Cluster.ClusterID)

	r.initialized = true
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) getNodes(client client.Client) ([]*corev1.Node, error) {
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

	// check if service with the same name exists and is valid
	svcObj := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: obj.Name, Namespace: obj.Namespace}, svcObj)
	if err != nil {
		logger.Errorf("Failed to get service %s/%s: %v", obj.Namespace, obj.Name, err)
		return err
	}
	if svcObj.Spec.Type != corev1.ServiceTypeLoadBalancer && svcObj.Spec.Type != corev1.ServiceTypeNodePort {
		logger.Warnf("Service %s/%s is not LoadBalancer or NodePort type, return.", obj.Namespace, obj.Name)
		return nil
	}

	// check if obj have config cluster annotation
	if obj.Annotations == nil || obj.Annotations[consts.ConfigClusterIdAnnotation] == "" {
		logger.Warnf("Annotation `%s` is empty, return.", consts.ConfigClusterIdAnnotation)
		return nil
	}

	// get fleet id from label
	fleetID := obj.Labels[consts.FleetIDLabel]
	if fleetID == "" {
		logger.Errorf("Label `%s` is empty, return.", consts.FleetIDLabel)
		return nil
	}

	// set service annotation to glb object
	svcObj.Annotations = obj.Annotations
	loadBalancerBuilder, err := builder.NewModelBuilderByService(ctx, svcObj, r.annotationParser, r.Client,
		r.netwotkID, r.subnetID, r.networkCIDR,
		r.Config.Cluster.ClusterID,
		r.Config.Cluster.Region,
		fleetID,
		r.knownNodes,
		r.cniMode,
		r.defaultPackageID,
	)
	if err != nil {
		logger.Error("Failed to create loadbalancer builder: ", err)
		return err
	}

	// ignore reconcile
	if loadBalancerBuilder.IsIgnored() {
		logger.Info("Object is ignored")
		return nil
	}

	// create loadbalancer, update annotation and reconcile later
	if loadBalancerBuilder.GetLoadBalancerID() == "" {
		// ignore if this cluster isn't config cluster
		if loadBalancerBuilder.GetConfigClusterID() != r.Config.Cluster.ClusterID {
			logger.Infof("Wait config cluster create loadbalancer.")
			return nil
		}

		// check if loadbalancer with the generate name exists, if exists, update annotation and return
		// check the name that user specified in the annotation first
		lbName := loadBalancerBuilder.GetLoadBalancerName()
		if lbName == "" {
			lbName = loadBalancerBuilder.GetLoadBalancerDefaultName()
		}
		lb, err := r.Provider.GetGlobalLoadBalancerByName(ctx, lbName)
		if err != nil {
			return err
		}
		if lb == nil {
			// create loadbalancer. It `must` create lb, listener, pool at the same time
			lb, err = r.Provider.CreateGlobalLoadBalancer(ctx, loadBalancerBuilder.CreateLoadBalancerOptions())
			if err != nil {
				return err
			}
		}
		if lb == nil || lb.ID == "" {
			return errors.New("load balancer not have ID after find by name or create, need to retry")
		}

		// wait for loadbalancer active, if lb is error, delete it and return error
		if _, err := r.Provider.WaitGlobalLoadBalancerActive(ctx, lb.ID); err != nil {
			if err == provider.ErrorLoadBalancerStatusError {
				if err := r.Provider.DeleteGlobalLoadBalancer(ctx, lb.ID); err != nil {
					logger.Error("Failed to delete loadbalancer: ", err)
					return err
				}
				logger.Infof("Delete loadbalancer \"%s\" because of status error, recreate now.", lb.ID)
				return errs.NewRequeueNeeded("loadbalancer status is error, delete and recreate")
			}
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}

		// update object annotation, also trigger reconcile immediately
		updateObjectAnnotation(ctx, r.Client, obj, map[string]string{
			fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID): lb.ID,
		})
		return nil
	} else {
		if _, err := r.Provider.WaitGlobalLoadBalancerActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
			logger.Error("Failed to wait for loadbalancer active: ", err)
			return err
		}
	}

	// inspect current loadbalancer in portal to compare with the new one
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, loadBalancerBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser,
		r.Config.Cluster.ClusterID,
		fleetID,
		r.knownNodes, obj)
	if err != nil {
		logger.Error("Failed to get current loadbalancer: ", err)
		return err
	}

	// // get old object annotations
	// oldAnnotations := make(map[string]string)
	// if oldObj, ok := oldObjInterface.(*corev1.Service); ok {
	// 	oldAnnotations = oldObj.Annotations
	// }

	// build oldBuilder
	// var oldBuilder builder.OldModelBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, map[string]string{}, r.annotationParser)
	// if oldBuilder.GetLoadBalancerID() != loadBalancerBuilder.GetLoadBalancerID() && oldBuilder.GetLoadBalancerID() != "" {
	// 	oldBuilder = builder.NewOldModelBuilder(obj.Annotations, obj.Annotations, r.annotationParser)

	// 	// clean up old loadbalancer if exists
	// 	go func() {
	// 		oldObj, ok := oldObjInterface.(*corev1.Service)
	// 		if !ok {
	// 			return
	// 		}
	// 		err := r.subDeleteObject(ctx, oldObj, true, false, false)
	// 		if err != nil {
	// 			logger.Error("Failed to delete old loadbalancer: ", err)
	// 			return
	// 		}
	// 		logger.Infof("Successfully ensure delete tags for old loadbalancer %s.", oldBuilder.GetLoadBalancerID())
	// 	}()
	// }

	// // ensure tags
	// err = r.ensureTags(loadBalancerBuilder, currentBuilder, oldBuilder)
	// if err != nil {
	// 	logger.Error("Failed to ensure tags: ", err)
	// 	return err
	// }

	// // ensure package
	// if currentBuilder.GetPackageID() != loadBalancerBuilder.GetPackageID() &&
	// 	currentBuilder.GetPackageID() != "" &&
	// 	loadBalancerBuilder.GetPackageID() != "" {
	// 	logger.Infof("Need resize loadbalancer from package %s -> %s", currentBuilder.GetPackageID(), loadBalancerBuilder.GetPackageID())
	// 	if err := r.Provider.ResizeLoadBalancer(ctx, loadBalancerBuilder.GetLoadBalancerID(), loadBalancerBuilder.GetPackageID()); err != nil {
	// 		logger.Error("Failed to resize loadbalancer: ", err)
	// 		return err
	// 	}
	// 	if _, err := r.Provider.WaitForLBActive(ctx, loadBalancerBuilder.GetLoadBalancerID()); err != nil {
	// 		logger.Error("Failed to wait for loadbalancer active: ", err)
	// 		return err
	// 	}
	// }

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

	// // delete redundant listeners
	// err = currentBuilder.DeleteRedundantListeners(oldBuilder, loadBalancerBuilder)
	// if err != nil {
	// 	logger.Error("Failed to delete redundant listener: ", err)
	// 	return err
	// }

	// // delete redundant pools, should check if pool is used by other listeners then ignore
	// err = currentBuilder.DeleteRedundantPools(oldBuilder, loadBalancerBuilder)
	// if err != nil {
	// 	logger.Error("Failed to delete redundant pool: ", err)
	// 	return err
	// }

	// // update management annotations
	// if err := r.updateObjectAnnotation(ctx, obj, loadBalancerBuilder.GetManageAnnotation()); err != nil {
	// 	logger.Error("Failed to update management annotations: ", err)
	// 	return err
	// }

	// watch endpoint if target type is ip or cni mode is cilium native routing
	r.resourceDependant.Set(obj, loadBalancerBuilder.GetTargetType() == builder.TargetTypeIP ||
		r.cniMode == utils.CiliumNativeRouting)

	// get lb updated time and add to update tracker
	lb, err := r.Provider.GetGlobalLoadBalancerByID(ctx, loadBalancerBuilder.GetLoadBalancerID())
	if err != nil {
		logger.Error("Failed to get loadbalancer: ", err)
		return err
	}
	// r.UpdateTracker.AddService(loadBalancerBuilder.GetLoadBalancerID(), lb.UpdatedAt, obj)

	// update status
	if len(lb.Domains) > 0 {
		err = r.updateObjectStatus(ctx, obj, lb.Domains[0].Hostname)
		if err != nil {
			logger.Error("Failed to update status: ", err)
			return err
		}
	} else {
		logger.Warn("Why this loadbalancer has no domain???????????")
	}

	// ensure security group with mutex
	err = r.ensureSecurityGroup(currentBuilder, loadBalancerBuilder, oldBuilder)
	if err != nil {
		logger.Error("Failed to ensure security group: ", err)
		return err
	}

	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) ensureSecurityGroup(currentBuilder builder.LoadBalancerBuilder, loadBalancerBuilder builder.ModelBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureSecurityGroups(loadBalancerBuilder, oldBuilder)
	if err != nil {
		return err
	}
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) deleteObject(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer) error {
	logger := contexts.NewContext(ctx).Log()

	if !k8s.HasFinalizer(obj, consts.GLBFinalizer) {
		logger.Warn("Finalizer is not found, return.")
		return nil
	}

	err := r.subDeleteObject(ctx, obj, true, true, true)
	if err != nil {
		return err
	}

	if err := r.FinalizerManager.RemoveFinalizers(ctx, obj, consts.GLBFinalizer); err != nil {
		return err
	}
	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) subDeleteObject(ctx context.Context, obj *v1alpha1.VngcloudGlobalLoadBalancer,
	deletetag, deleteResource, deleteSegroup bool) error {
	logger := contexts.NewContext(ctx).Log()

	// build oldBuilder
	oldBuilder := builder.NewOldModelBuilder(obj.Annotations, obj.Annotations, r.annotationParser)

	// ignore reconcile
	if oldBuilder.IsIgnored() {
		logger.Info("Ingress is ignored")
		return nil
	}

	if oldBuilder.GetLoadBalancerID() == "" {
		logger.Info("LoadBalancer ID is empty, return.")
		return nil
	}

	// // remove from update tracker
	// r.UpdateTracker.RemoveIngress(oldBuilder.GetLoadBalancerID(), obj)

	// get fleet id from label
	fleetID := obj.Labels[consts.FleetIDLabel]
	if fleetID == "" {
		logger.Errorf("Label `%s` is empty, return.", consts.FleetIDLabel)
		return nil
	}

	// inspect current loadbalancer in portal to compare with
	currentBuilder, err := builder.NewLoadBalancerBuilderByLoadBalancerID(ctx, oldBuilder.GetLoadBalancerID(),
		r.Provider, r.annotationParser,
		r.Config.Cluster.ClusterID,
		fleetID,
		r.knownNodes, obj)

	if err != nil {
		if errs.IsGlobalLoadBalancerNotFound(err) {
			return nil
		}
		logger.Error("Failed to get current loadbalancer: ", err)
		return err
	}

	// // build loadbalancer model, pass nil to object to create a model with default values
	// newBuilder, err := builder.NewModelBuilderByIngress(ctx, nil, r.annotationParser, r.Client,
	// 	r.networkID, r.subnetID, r.subnetCIDR,
	// 	r.Config.Cluster.ClusterID,
	// 	r.knownNodes,
	// 	r.cniMode,
	// 	r.defaultPackageID,
	// )
	// if err != nil {
	// 	logger.Error("Failed to create new model builder: ", err)
	// 	return err
	// }

	// if deletetag {
	// 	// ensure delete tags
	// 	err = r.ensureDeleteTags(currentBuilder, oldBuilder)
	// 	if err != nil {
	// 		logger.Error("Failed to ensure delete tags: ", err)
	// 		return err
	// 	}
	// }

	if deleteResource {
		if err := r.Provider.DeleteGlobalLoadBalancer(ctx, oldBuilder.GetLoadBalancerID()); err != nil {
			if !errs.IsGlobalLoadBalancerNotFound(err) {
				logger.Error("Failed to delete loadbalancer: ", err)
				return err
			}
		}
	}

	if deleteSegroup {
		// ensure delete security group with mutex
		err = r.ensureDeleteSecurityGroup(currentBuilder, oldBuilder)
		if err != nil {
			logger.Error("Failed to ensure delete security group: ", err)
			return err
		}
	}

	return nil
}

func (r *VngcloudGlobalLoadBalancerReconciler) ensureDeleteSecurityGroup(currentBuilder builder.LoadBalancerBuilder, oldBuilder builder.OldModelBuilder) error {
	secGroupMutex.Lock()
	defer secGroupMutex.Unlock()
	err := currentBuilder.EnsureDeleteSecurityGroups(oldBuilder)
	if err != nil {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VngcloudGlobalLoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1alpha1.VngcloudGlobalLoadBalancer{}).
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
				objList := &v1alpha1.VngcloudGlobalLoadBalancerList{}
				err := r.List(ctx, objList)
				if err != nil {
					return []reconcile.Request{}
				}

				// filter
				requests := make([]reconcile.Request, 0)
				for _, obj := range objList.Items {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      obj.GetName(),
							Namespace: obj.GetNamespace(),
						},
					})
				}
				return requests
			}),
			k8sBuilder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5, // ..................
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				switch e.Object.(type) {
				case *v1alpha1.VngcloudGlobalLoadBalancer:
					logrus.Info("Detect create VGLB event.")
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
				case *v1alpha1.VngcloudGlobalLoadBalancer:
					oldObj := e.ObjectOld.(*v1alpha1.VngcloudGlobalLoadBalancer)
					newObj := e.ObjectNew.(*v1alpha1.VngcloudGlobalLoadBalancer)

					if !reflect.DeepEqual(oldObj.Spec, newObj.Spec) {
						logrus.Info("Detect update VGLB Spec event.")
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
						logrus.Info("Detect update VGLB Annotations event.")
						debugCompareMapString(oldAnnotations, newAnnotations)
						return true
					}
					if !reflect.DeepEqual(oldObj.DeletionTimestamp.IsZero(), newObj.DeletionTimestamp.IsZero()) {
						logrus.Info("Detect update VGLB DeletionTimestamp event.")
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
					logrus.Info("Detect update Node event.")
					r.knownNodes, _ = r.getNodes(r.Client)
					return true
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
				case *v1alpha1.VngcloudGlobalLoadBalancer:
					logrus.Info("Detect delete VGLB event.")
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

func (r *VngcloudGlobalLoadBalancerReconciler) updateObjectStatus(ctx context.Context, obj client.Object, address string) error {
	if obj == nil {
		return nil
	}
	if address == "" {
		return errors.New("address is empty")
	}

	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("Update status for object %s/%s = %s", obj.GetNamespace(), obj.GetName(), address)

	// get object again to avoid conflict
	object := &v1alpha1.VngcloudGlobalLoadBalancer{}
	err := r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, object)
	if err != nil {
		logger.Error("Failed to get object: ", err)
		return err
	}
	objectOld := object.DeepCopy()
	object.Status.Address = address

	return r.Status().Patch(ctx, object, client.MergeFrom(objectOld))
}
