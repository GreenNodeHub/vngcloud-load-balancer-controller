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
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/nsg"
)

// NewEnqueueRequestForNsgEvent constructs new enqueueRequestsForNsgEvent.
func NewEnqueueRequestForNsgEvent(eventRecorder record.EventRecorder,
	nsgUtils nsg.NodeSecurityGroupUtils) *enqueueRequestsForNsgEvent {
	return &enqueueRequestsForNsgEvent{
		eventRecorder: eventRecorder,
		nsgUtils:      nsgUtils,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForNsgEvent)(nil)

type enqueueRequestsForNsgEvent struct {
	eventRecorder record.EventRecorder
	nsgUtils      nsg.NodeSecurityGroupUtils
}

func (h *enqueueRequestsForNsgEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueManagedObject(ctx, queue, e.Object.(*v1alpha1.NodeSecurityGroup))
}

func (h *enqueueRequestsForNsgEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldObj := e.ObjectOld.(*v1alpha1.NodeSecurityGroup)
	newObj := e.ObjectNew.(*v1alpha1.NodeSecurityGroup)

	if !equality.Semantic.DeepEqual(oldObj.ResourceVersion, newObj.ResourceVersion) {
		if equality.Semantic.DeepEqual(oldObj.Annotations, newObj.Annotations) &&
			equality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) &&
			equality.Semantic.DeepEqual(oldObj.DeletionTimestamp.IsZero(), newObj.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueManagedObject(ctx, queue, newObj)
}

func (h *enqueueRequestsForNsgEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
	// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object. Since
	// deletion is already taken care of during update event, we will ignore this event.
}

func (h *enqueueRequestsForNsgEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForNsgEvent) enqueueManagedObject(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], obj *v1alpha1.NodeSecurityGroup) {
	// Check if the obj needs to be handled
	if !h.nsgUtils.IsPendingFinalization(obj) && !h.nsgUtils.IsSupported(obj) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: obj.Namespace,
			Name:      obj.Name,
		},
	})
}
