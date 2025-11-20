package eventhandlers

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/nsg"
)

// NewEnqueueRequestForNodeEvent constructs new enqueueRequestsForNodeEvent.
func NewEnqueueRequestForNodeEvent(k8sClient client.Client,
	nsgUtils nsg.NodeSecurityGroupUtils, logger logr.Logger) *enqueueRequestsForNodeEvent {
	return &enqueueRequestsForNodeEvent{
		k8sClient: k8sClient,
		nsgUtils:  nsgUtils,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForNodeEvent)(nil)

type enqueueRequestsForNodeEvent struct {
	k8sClient client.Client
	nsgUtils  nsg.NodeSecurityGroupUtils
	logger    logr.Logger
}

func (h *enqueueRequestsForNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Create Node", "name", e.Object.GetName())
	h.enqueueAllNsg(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Update Node", "name", e.ObjectNew.GetName())
	h.enqueueAllNsg(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.logger.V(1).Info("Delete Node", "name", e.Object.GetName())
	h.enqueueAllNsg(ctx, queue)
}

func (h *enqueueRequestsForNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForNodeEvent) enqueueAllNsg(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List all
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	if err := h.k8sClient.List(ctx, nsgList); err != nil {
		return
	}

	// Enqueue all supported nsgs
	for _, nsg := range nsgList.Items {
		if !h.nsgUtils.IsPendingFinalization(&nsg) && !h.nsgUtils.IsSupported(&nsg) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&nsg),
		})
	}
}
