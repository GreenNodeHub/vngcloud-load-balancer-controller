package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
)

// NewEnqueueRequestForIngressEvent constructs new enqueueRequestsForIngressEvent.
func NewEnqueueRequestForIngressEvent(eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForIngressEvent {
	return &enqueueRequestsForIngressEvent{
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForIngressEvent)(nil)

type enqueueRequestsForIngressEvent struct {
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForIngressEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueManagedIngress(ctx, queue, e.Object.(*networkingv1.Ingress))
}

func (h *enqueueRequestsForIngressEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldIng := e.ObjectOld.(*networkingv1.Ingress)
	newIng := e.ObjectNew.(*networkingv1.Ingress)

	// we only care below update event:
	//	1. Ingress annotation updates
	//	2. Ingress spec updates
	//	3. Ingress deletion
	if !equality.Semantic.DeepEqual(oldIng.ResourceVersion, newIng.ResourceVersion) {
		if equality.Semantic.DeepEqual(oldIng.Annotations, newIng.Annotations) &&
			equality.Semantic.DeepEqual(oldIng.Spec, newIng.Spec) &&
			equality.Semantic.DeepEqual(oldIng.DeletionTimestamp.IsZero(), newIng.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueManagedIngress(ctx, queue, newIng)
}

func (h *enqueueRequestsForIngressEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// since we'll always attach an finalizer before doing any reconcile action,
	// user triggered delete action will actually be an update action with deletionTimestamp set,
	// which will be handled by update event handler.
	// so we'll just ignore delete events to avoid unnecessary reconcile call.
}

func (h *enqueueRequestsForIngressEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForIngressEvent) enqueueManagedIngress(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], ingress *networkingv1.Ingress) {
	// Check if the ing needs to be handled
	if !h.ingressUtils.IsIngressPendingFinalization(ingress) && !h.ingressUtils.IsIngressSupported(ingress) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ingress.Namespace,
			Name:      ingress.Name,
		},
	})
}
