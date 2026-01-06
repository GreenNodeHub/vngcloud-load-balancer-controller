package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
)

// NewEnqueueRequestForNodeEvent constructs new enqueueRequestsForNodeEvent.
func NewEnqueueRequestForNodeEvent(k8sClient client.Client,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForNodeEvent {
	return &enqueueRequestsForNodeEvent{
		k8sClient:    k8sClient,
		ingressUtils: ingressUtils,
		logger:       logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForNodeEvent)(nil)

type enqueueRequestsForNodeEvent struct {
	k8sClient    client.Client
	ingressUtils ingress.IngressUtils
	logger       logr.Logger
}

func (h *enqueueRequestsForNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.Info("node created, enqueuing all ingresss", "node", node.Name)
	h.enqueueAllSupportedIngresss(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldNode := e.ObjectOld.(*corev1.Node)
	newNode := e.ObjectNew.(*corev1.Node)

	// Skip reconciliation if only unimportant fields changed
	// Only reconcile if labels, spec, addresses, or ready condition changed
	if equality.Semantic.DeepEqual(oldNode.Labels, newNode.Labels) &&
		equality.Semantic.DeepEqual(oldNode.Spec, newNode.Spec) &&
		equality.Semantic.DeepEqual(oldNode.Status.Addresses, newNode.Status.Addresses) &&
		getNodeReadyCondition(oldNode) == getNodeReadyCondition(newNode) {
		return
	}

	h.logger.Info("node updated, enqueuing all ingresss", "node", newNode.Name)
	h.enqueueAllSupportedIngresss(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.Info("node deleted, enqueuing all ingresss", "node", node.Name)
	h.enqueueAllSupportedIngresss(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForNodeEvent) enqueueAllSupportedIngresss(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List all ingresss
	svcList := &networkingv1.IngressList{}
	if err := h.k8sClient.List(ctx, svcList); err != nil {
		h.logger.Error(err, "failed to list ingresss for node event")
		return
	}

	// Enqueue all supported ingresss
	for _, svc := range svcList.Items {
		if !h.ingressUtils.IsIngressPendingFinalization(&svc) && !h.ingressUtils.IsIngressSupported(&svc) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&svc),
		})
	}
}

func getNodeReadyCondition(node *corev1.Node) corev1.ConditionStatus {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status
		}
	}
	return corev1.ConditionUnknown
}
