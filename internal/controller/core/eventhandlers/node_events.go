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
)

// NewEnqueueRequestForNodeEvent constructs new enqueueRequestsForNodeEvent.
func NewEnqueueRequestForNodeEvent(k8sClient client.Client, logger logr.Logger) *enqueueRequestsForNodeEvent {
	return &enqueueRequestsForNodeEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForNodeEvent)(nil)

type enqueueRequestsForNodeEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (h *enqueueRequestsForNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.Info("node created, enqueuing all LoadBalancer services", "node", node.Name)
	h.enqueueAllLoadBalancerServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	newNode := e.ObjectNew.(*corev1.Node)

	h.logger.Info("node updated, enqueuing all LoadBalancer services", "node", newNode.Name)
	h.enqueueAllLoadBalancerServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.Info("node deleted, enqueuing all LoadBalancer services", "node", node.Name)
	h.enqueueAllLoadBalancerServices(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForNodeEvent) enqueueAllLoadBalancerServices(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List all services of type LoadBalancer
	svcList := &corev1.ServiceList{}
	if err := h.k8sClient.List(ctx, svcList); err != nil {
		h.logger.Error(err, "failed to list services for node event")
		return
	}

	for _, svc := range svcList.Items {
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			queue.Add(reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&svc),
			})
		}
	}
}
