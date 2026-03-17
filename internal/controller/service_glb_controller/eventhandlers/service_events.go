package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	service_glb "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service_glb"
)

// NewEnqueueRequestForServiceGLBEvent constructs a new enqueueRequestsForServiceGLBEvent.
func NewEnqueueRequestForServiceGLBEvent(
	eventRecorder record.EventRecorder,
	serviceGLBUtils service_glb.ServiceGLBUtils,
	annotationParser annotations.Parser,
	logger logr.Logger,
) *enqueueRequestsForServiceGLBEvent {
	return &enqueueRequestsForServiceGLBEvent{
		eventRecorder:    eventRecorder,
		serviceGLBUtils:  serviceGLBUtils,
		annotationParser: annotationParser,
		logger:           logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceGLBEvent)(nil)

type enqueueRequestsForServiceGLBEvent struct {
	eventRecorder    record.EventRecorder
	serviceGLBUtils  service_glb.ServiceGLBUtils
	annotationParser annotations.Parser
	logger           logr.Logger
}

func (h *enqueueRequestsForServiceGLBEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueManagedService(ctx, queue, e.Object.(*corev1.Service))
}

func (h *enqueueRequestsForServiceGLBEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
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

	// Enqueue if new state is GLB-relevant OR old had annotation (annotation removal triggers cleanup)
	if h.serviceGLBUtils.IsServiceGLBPendingFinalization(newSvc) ||
		h.serviceGLBUtils.IsServiceGLBSupported(newSvc) ||
		h.hadGLBAnnotation(oldSvc) {
		h.logger.V(1).Info("Enqueue Service", "namespace", newSvc.GetNamespace(), "name", newSvc.GetName())
		queue.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: newSvc.Namespace, Name: newSvc.Name}})
	}
}

func (h *enqueueRequestsForServiceGLBEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
	// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object.
	// Since deletion is already taken care of during update event, we ignore this event.
}

func (h *enqueueRequestsForServiceGLBEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForServiceGLBEvent) enqueueManagedService(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], svc *corev1.Service) {
	if !h.serviceGLBUtils.IsServiceGLBPendingFinalization(svc) && !h.serviceGLBUtils.IsServiceGLBSupported(svc) {
		return
	}
	h.logger.V(1).Info("Enqueue Service", "namespace", svc.Namespace, "name", svc.Name)
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: svc.Namespace,
			Name:      svc.Name,
		},
	})
}

// hadGLBAnnotation returns true if the service previously had the GLB enable annotation set to true.
// Used to detect annotation removal so that cleanup (reconcileDelete) can be triggered.
func (h *enqueueRequestsForServiceGLBEvent) hadGLBAnnotation(svc *corev1.Service) bool {
	enabled := false
	h.annotationParser.ParseBoolAnnotation(annotations.SuffixGLBEnable, &enabled, svc.Annotations)
	return enabled
}
