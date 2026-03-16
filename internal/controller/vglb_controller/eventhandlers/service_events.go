package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/vglb"
)

// NewEnqueueRequestForVglbServiceEvent constructs new enqueueRequestsForVglbServiceEvent.
func NewEnqueueRequestForVglbServiceEvent(k8sClient client.Client,
	vglbUtils vglb.VngcloudGlobalLoadBalancerUtils, logger logr.Logger) *enqueueRequestsForVglbServiceEvent {
	return &enqueueRequestsForVglbServiceEvent{
		k8sClient: k8sClient,
		vglbUtils: vglbUtils,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForVglbServiceEvent)(nil)

type enqueueRequestsForVglbServiceEvent struct {
	k8sClient client.Client
	vglbUtils vglb.VngcloudGlobalLoadBalancerUtils
	logger    logr.Logger
}

func (h *enqueueRequestsForVglbServiceEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svc := e.Object.(*corev1.Service)
	h.logger.V(1).Info("Create Service", "namespace", svc.GetNamespace(), "name", svc.GetName())
	h.enqueueSameNameVglb(ctx, queue, svc)
}

func (h *enqueueRequestsForVglbServiceEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSvc := e.ObjectOld.(*corev1.Service)
	newSvc := e.ObjectNew.(*corev1.Service)

	// Allow periodic sync events (same resource version) for drift detection
	isSyncEvent := oldSvc.GetResourceVersion() == newSvc.GetResourceVersion()
	if !isSyncEvent &&
		equality.Semantic.DeepEqual(oldSvc.Annotations, newSvc.Annotations) &&
		equality.Semantic.DeepEqual(oldSvc.Spec, newSvc.Spec) &&
		oldSvc.DeletionTimestamp.IsZero() == newSvc.DeletionTimestamp.IsZero() {
		return
	}

	h.logger.V(1).Info("Update Service", "namespace", newSvc.GetNamespace(), "name", newSvc.GetName())
	h.enqueueSameNameVglb(ctx, queue, newSvc)
}

func (h *enqueueRequestsForVglbServiceEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	svc := e.Object.(*corev1.Service)
	h.logger.V(1).Info("Delete Service", "namespace", svc.GetNamespace(), "name", svc.GetName())
	// Service deletion should trigger VGLB reconcile so it enters the "service not found" requeue path
	h.enqueueSameNameVglb(ctx, queue, svc)
}

func (h *enqueueRequestsForVglbServiceEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForVglbServiceEvent) enqueueSameNameVglb(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], svc *corev1.Service) {
	// Look up VGLB with same name+namespace as the service
	vglbObj := &v1alpha1.VngcloudGlobalLoadBalancer{}
	if err := h.k8sClient.Get(ctx, types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, vglbObj); err != nil {
		// If not found, return silently (no VGLB for this service)
		return
	}

	// Check if the VGLB needs to be handled
	if !h.vglbUtils.IsPendingFinalization(vglbObj) && !h.vglbUtils.IsSupported(vglbObj) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: vglbObj.Namespace,
			Name:      vglbObj.Name,
		},
	})
}
