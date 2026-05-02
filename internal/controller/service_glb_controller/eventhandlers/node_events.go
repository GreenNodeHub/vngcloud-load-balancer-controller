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

	service_glb "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service_glb"
)

// NewEnqueueRequestForServiceGLBNodeEvent constructs a new enqueueRequestsForServiceGLBNodeEvent.
func NewEnqueueRequestForServiceGLBNodeEvent(
	k8sClient client.Client,
	serviceGLBUtils service_glb.ServiceGLBUtils,
	logger logr.Logger,
) *enqueueRequestsForServiceGLBNodeEvent {
	return &enqueueRequestsForServiceGLBNodeEvent{
		k8sClient:       k8sClient,
		serviceGLBUtils: serviceGLBUtils,
		logger:          logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceGLBNodeEvent)(nil)

type enqueueRequestsForServiceGLBNodeEvent struct {
	k8sClient       client.Client
	serviceGLBUtils service_glb.ServiceGLBUtils
	logger          logr.Logger
}

func (h *enqueueRequestsForServiceGLBNodeEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueAllGLBServices(ctx, queue)
}

func (h *enqueueRequestsForServiceGLBNodeEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldNode := e.ObjectOld.(*corev1.Node)
	newNode := e.ObjectNew.(*corev1.Node)

	// Skip reconciliation if only unimportant fields changed.
	// Only reconcile if labels, spec, addresses, or ready condition changed.
	if equality.Semantic.DeepEqual(oldNode.Labels, newNode.Labels) &&
		equality.Semantic.DeepEqual(oldNode.Spec, newNode.Spec) &&
		equality.Semantic.DeepEqual(oldNode.Status.Addresses, newNode.Status.Addresses) &&
		getNodeReadyCondition(oldNode) == getNodeReadyCondition(newNode) {
		return
	}

	h.enqueueAllGLBServices(ctx, queue)
}

func (h *enqueueRequestsForServiceGLBNodeEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueAllGLBServices(ctx, queue)
}

func (h *enqueueRequestsForServiceGLBNodeEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForServiceGLBNodeEvent) enqueueAllGLBServices(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// List ALL Services cluster-wide (annotations are not indexable, so no label/field selector)
	svcList := &corev1.ServiceList{}
	if err := h.k8sClient.List(ctx, svcList); err != nil {
		h.logger.Error(err, "failed to list Services for node event")
		return
	}

	for _, svc := range svcList.Items {
		svcCopy := svc
		if !h.serviceGLBUtils.IsServiceGLBPendingFinalization(&svcCopy) && !h.serviceGLBUtils.IsServiceGLBSupported(&svcCopy) {
			continue
		}
		h.logger.V(1).Info("Enqueue Service", "namespace", svcCopy.Namespace, "name", svcCopy.Name)
		queue.Add(reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: svcCopy.Namespace,
				Name:      svcCopy.Name,
			},
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
