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
)

// NewEnqueueRequestForEndpointEvent constructs new enqueueRequestsForEndpointEvent.
func NewEnqueueRequestForEndpointEvent(k8sClient client.Client, logger logr.Logger) *enqueueRequestsForEndpointEvent {
	return &enqueueRequestsForEndpointEvent{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForEndpointEvent)(nil)

type enqueueRequestsForEndpointEvent struct {
	k8sClient client.Client
	logger    logr.Logger
}

func (h *enqueueRequestsForEndpointEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	endpoint := e.Object.(*corev1.Endpoints)
	h.enqueueServiceForEndpoint(ctx, queue, endpoint)
}

func (h *enqueueRequestsForEndpointEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldEndpoint := e.ObjectOld.(*corev1.Endpoints)
	newEndpoint := e.ObjectNew.(*corev1.Endpoints)

	// Only reconcile if the endpoint subsets have changed
	if !equality.Semantic.DeepEqual(oldEndpoint.Subsets, newEndpoint.Subsets) {
		h.enqueueServiceForEndpoint(ctx, queue, newEndpoint)
	}
}

func (h *enqueueRequestsForEndpointEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	endpoint := e.Object.(*corev1.Endpoints)
	h.enqueueServiceForEndpoint(ctx, queue, endpoint)
}

func (h *enqueueRequestsForEndpointEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForEndpointEvent) enqueueServiceForEndpoint(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], endpoint *corev1.Endpoints) {
	// Get the service with the same name as the endpoint
	svcKey := types.NamespacedName{
		Namespace: endpoint.Namespace,
		Name:      endpoint.Name,
	}

	// Check if the corresponding service exists and is a LoadBalancer
	svc := &corev1.Service{}
	if err := h.k8sClient.Get(ctx, svcKey, svc); err != nil {
		if client.IgnoreNotFound(err) != nil {
			h.logger.Error(err, "failed to get service for endpoint", "endpoint", svcKey)
		}
		return
	}

	// Only enqueue if the service is of type LoadBalancer
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		h.logger.Info("endpoint changed, enqueuing service for reconciliation",
			"service", svcKey.String(),
			"endpointSubsets", len(endpoint.Subsets))
		queue.Add(reconcile.Request{
			NamespacedName: svcKey,
		})
	}
}