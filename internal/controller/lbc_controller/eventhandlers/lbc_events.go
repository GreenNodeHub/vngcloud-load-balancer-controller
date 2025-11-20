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

	"github.com/go-logr/logr"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/lbc"
)

// NewEnqueueRequestForLbcEvent constructs new enqueueRequestsForLbcEvent.
func NewEnqueueRequestForLbcEvent(eventRecorder record.EventRecorder,
	lbcUtils lbc.LoadBalancerConfigUtils, logger logr.Logger) *enqueueRequestsForLbcEvent {
	return &enqueueRequestsForLbcEvent{
		eventRecorder: eventRecorder,
		lbcUtils:      lbcUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForLbcEvent)(nil)

type enqueueRequestsForLbcEvent struct {
	eventRecorder record.EventRecorder
	lbcUtils      lbc.LoadBalancerConfigUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForLbcEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Create LBC", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
	h.enqueueManagedObject(ctx, queue, e.Object.(*v1alpha1.LoadBalancerConfig))
}

func (h *enqueueRequestsForLbcEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldObj := e.ObjectOld.(*v1alpha1.LoadBalancerConfig)
	newObj := e.ObjectNew.(*v1alpha1.LoadBalancerConfig)

	// Skip reconciliation if only unimportant fields changed
	if equality.Semantic.DeepEqual(oldObj.Annotations, newObj.Annotations) &&
		equality.Semantic.DeepEqual(oldObj.Spec, newObj.Spec) &&
		oldObj.DeletionTimestamp.IsZero() == newObj.DeletionTimestamp.IsZero() {
		return
	}

	h.logger.V(1).Info("Update LBC", "namespace", newObj.GetNamespace(), "name", newObj.GetName())
	h.enqueueManagedObject(ctx, queue, newObj)
}

func (h *enqueueRequestsForLbcEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// We attach a finalizer during reconcile, and handle the user triggered delete action during the update event.
	// In case of delete, there will first be an update event with nonzero deletionTimestamp set on the object. Since
	// deletion is already taken care of during update event, we will ignore this event.
}

func (h *enqueueRequestsForLbcEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForLbcEvent) enqueueManagedObject(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], object *v1alpha1.LoadBalancerConfig) {
	// Check if the obj needs to be handled
	if !h.lbcUtils.IsPendingFinalization(object) && !h.lbcUtils.IsSupported(object) {
		return
	}
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: object.Namespace,
			Name:      object.Name,
		},
	})
}
