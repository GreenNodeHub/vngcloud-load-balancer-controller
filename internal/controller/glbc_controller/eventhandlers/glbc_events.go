package eventhandlers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/glbc"
)

// NewEnqueueRequestForGlbcEvent constructs new enqueueRequestsForGlbcEvent.
func NewEnqueueRequestForGlbcEvent(eventRecorder record.EventRecorder,
	glbcUtils glbc.GlobalLoadBalancerConfigUtils) *enqueueRequestsForGlbcEvent {
	return &enqueueRequestsForGlbcEvent{
		eventRecorder: eventRecorder,
		glbcUtils:     glbcUtils,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForGlbcEvent)(nil)

type enqueueRequestsForGlbcEvent struct {
	eventRecorder record.EventRecorder
	glbcUtils     glbc.GlobalLoadBalancerConfigUtils
}

func (h *enqueueRequestsForGlbcEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueManagedObject(ctx, queue, e.Object.(*v1alpha1.GlobalLoadBalancerConfig))
}

func (h *enqueueRequestsForGlbcEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldObj := e.ObjectOld.(*v1alpha1.GlobalLoadBalancerConfig)
	newObj := e.ObjectNew.(*v1alpha1.GlobalLoadBalancerConfig)

	// Skip reconciliation if only unimportant fields changed
	if equality.Semantic.DeepEqual(oldObj.Annotations, newObj.Annotations) &&
		equality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) &&
		oldObj.DeletionTimestamp.IsZero() == newObj.DeletionTimestamp.IsZero() {
		return
	}

	h.enqueueManagedObject(ctx, queue, newObj)
}

func (h *enqueueRequestsForGlbcEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
	// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object. Since
	// deletion is already taken care of during update event, we will ignore this event.
}

func (h *enqueueRequestsForGlbcEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForGlbcEvent) enqueueManagedObject(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], object *v1alpha1.GlobalLoadBalancerConfig) {
	// Check if the obj needs to be handled
	if !h.glbcUtils.IsPendingFinalization(object) && !h.glbcUtils.IsSupported(object) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: object.Namespace,
			Name:      object.Name,
		},
	})
}
