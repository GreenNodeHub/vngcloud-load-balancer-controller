package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/vglb"
)

// NewEnqueueRequestForVglbNodeEvent constructs new enqueueRequestsForVglbNodeEvent.
func NewEnqueueRequestForVglbNodeEvent(k8sClient client.Client,
	vglbUtils vglb.VngcloudGlobalLoadBalancerUtils, logger logr.Logger) *enqueueRequestsForVglbNodeEvent {
	return &enqueueRequestsForVglbNodeEvent{
		k8sClient: k8sClient,
		vglbUtils: vglbUtils,
		logger:    logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForVglbNodeEvent)(nil)

type enqueueRequestsForVglbNodeEvent struct {
	k8sClient client.Client
	vglbUtils vglb.VngcloudGlobalLoadBalancerUtils
	logger    logr.Logger
}

func (h *enqueueRequestsForVglbNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.V(1).Info("Create Node", "name", node.Name)
	h.enqueueAllVglbs(ctx, queue)
}

func (h *enqueueRequestsForVglbNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
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

	h.logger.V(1).Info("Update Node", "name", newNode.Name)
	h.enqueueAllVglbs(ctx, queue)
}

func (h *enqueueRequestsForVglbNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	node := e.Object.(*corev1.Node)
	h.logger.V(1).Info("Delete Node", "name", node.Name)
	h.enqueueAllVglbs(ctx, queue)
}

func (h *enqueueRequestsForVglbNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForVglbNodeEvent) enqueueAllVglbs(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List all VGLBs
	vglbList := &v1alpha1.VngcloudGlobalLoadBalancerList{}
	if err := h.k8sClient.List(ctx, vglbList); err != nil {
		h.logger.Error(err, "failed to list VngcloudGlobalLoadBalancers for node event")
		return
	}

	// Enqueue all managed VGLBs
	for _, item := range vglbList.Items {
		if !h.vglbUtils.IsPendingFinalization(&item) && !h.vglbUtils.IsSupported(&item) {
			continue
		}
		queue.Add(reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
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
