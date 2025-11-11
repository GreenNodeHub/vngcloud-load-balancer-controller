package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
)

func NewEnqueueRequestsForIngressEvent(eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils,
	logger logr.Logger) handler.TypedEventHandler[*networking.Ingress, reconcile.Request] {
	return &enqueueRequestsForIngressEvent{
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.TypedEventHandler[*networking.Ingress, reconcile.Request] = (*enqueueRequestsForIngressEvent)(nil)

type enqueueRequestsForIngressEvent struct {
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForIngressEvent) Create(ctx context.Context, e event.TypedCreateEvent[*networking.Ingress], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueIngress(queue, e.Object)
}

func (h *enqueueRequestsForIngressEvent) Update(ctx context.Context, e event.TypedUpdateEvent[*networking.Ingress], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	ingOld := e.ObjectOld
	ingNew := e.ObjectNew

	// we only care below update event:
	//	1. Ingress annotation updates
	//	2. Ingress spec updates
	//	3. Ingress deletion
	if !equality.Semantic.DeepEqual(ingOld.ResourceVersion, ingNew.ResourceVersion) {
		if equality.Semantic.DeepEqual(ingOld.Annotations, ingNew.Annotations) &&
			equality.Semantic.DeepEqual(ingOld.Spec, ingNew.Spec) &&
			equality.Semantic.DeepEqual(ingOld.DeletionTimestamp.IsZero(), ingNew.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueIngress(queue, ingNew)
}

func (h *enqueueRequestsForIngressEvent) Delete(ctx context.Context, e event.TypedDeleteEvent[*networking.Ingress], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// since we'll always attach an finalizer before doing any reconcile action,
	// user triggered delete action will actually be an update action with deletionTimestamp set,
	// which will be handled by update event handler.
	// so we'll just ignore delete events to avoid unnecessary reconcile call.
}

func (h *enqueueRequestsForIngressEvent) Generic(ctx context.Context, e event.TypedGenericEvent[*networking.Ingress], queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForIngressEvent) enqueueIngress(queue workqueue.TypedRateLimitingInterface[reconcile.Request], ingress *networking.Ingress) {
	// Check if the ingress needs to be handled
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
