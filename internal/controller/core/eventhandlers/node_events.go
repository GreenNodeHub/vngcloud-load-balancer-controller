package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
)

// NewEnqueueRequestForNodeEvent constructs new enqueueRequestsForNodeEvent.
func NewEnqueueRequestForNodeEvent(k8sClient client.Client,
	serviceUtils service.ServiceUtils, logger logr.Logger) *enqueueRequestsForNodeEvent {
	return &enqueueRequestsForNodeEvent{
		k8sClient:    k8sClient,
		serviceUtils: serviceUtils,
		logger:       logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForNodeEvent)(nil)

type enqueueRequestsForNodeEvent struct {
	k8sClient    client.Client
	serviceUtils service.ServiceUtils
	logger       logr.Logger
}

func (h *enqueueRequestsForNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.V(1).Info("Create Node", "name", node.Name)
	h.enqueueAllSupportedServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	newNode := e.ObjectNew.(*corev1.Node)
	h.logger.V(1).Info("Update Node", "name", newNode.Name)
	h.enqueueAllSupportedServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.V(1).Info("Delete Node", "name", node.Name)
	h.enqueueAllSupportedServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForNodeEvent) enqueueAllSupportedServices(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List all services
	svcList := &corev1.ServiceList{}
	if err := h.k8sClient.List(ctx, svcList); err != nil {
		h.logger.Error(err, "failed to list services for node event")
		return
	}

	// Enqueue all supported services
	for _, svc := range svcList.Items {
		if !h.serviceUtils.IsServicePendingFinalization(&svc) && !h.serviceUtils.IsServiceSupported(&svc) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&svc),
		})
	}
}
