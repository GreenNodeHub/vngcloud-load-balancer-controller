package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
)

// NewEnqueueRequestForEndpointsEvent constructs new enqueueRequestsForEndpointsEvent.
func NewEnqueueRequestForEndpointsEvent(k8sClient client.Client, eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForEndpointsEvent {
	return &enqueueRequestsForEndpointsEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForEndpointsEvent)(nil)

type enqueueRequestsForEndpointsEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForEndpointsEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Create Endpoints", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Endpoints))
}

func (h *enqueueRequestsForEndpointsEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldEndpoint := e.ObjectOld.(*corev1.Endpoints)
	newEndpoint := e.ObjectNew.(*corev1.Endpoints)

	// Only reconcile if the endpoint subsets have changed
	if !equality.Semantic.DeepEqual(oldEndpoint.Subsets, newEndpoint.Subsets) {
		h.logger.V(1).Info("Update Endpoints", "namespace", newEndpoint.GetNamespace(), "name", newEndpoint.GetName())
		h.enqueueImpactedIngresses(ctx, queue, newEndpoint)
	}
}

func (h *enqueueRequestsForEndpointsEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Delete Endpoints", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Endpoints))
}

func (h *enqueueRequestsForEndpointsEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedIngresses(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], ep *corev1.Endpoints) {
	ingList := &networkingv1.IngressList{}
	if err := h.k8sClient.List(ctx, ingList,
		client.InNamespace(ep.GetNamespace()),
		// use service reference index to find impacted ingresses
		client.MatchingFields{ingress.IndexKeyServiceRefName: ep.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	// enqueue all impacted ingresses if supported
	for index := range ingList.Items {
		ing := &ingList.Items[index]

		if !h.ingressUtils.IsIngressPendingFinalization(ing) && !h.ingressUtils.IsIngressSupported(ing) {
			continue
		}

		h.logger.V(1).Info("enqueue ingress for endpoints event",
			"ingress", types.NamespacedName{
				Namespace: ing.Namespace,
				Name:      ing.Name,
			}.String(),
			"endpoints", types.NamespacedName{
				Namespace: ep.Namespace,
				Name:      ep.Name,
			}.String(),
		)
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: ing.Namespace,
				Name:      ing.Name,
			},
		})
	}
}
