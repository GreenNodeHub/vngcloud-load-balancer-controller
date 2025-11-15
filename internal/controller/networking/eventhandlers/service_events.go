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

// NewEnqueueRequestForServiceEvent constructs new enqueueRequestsForServiceEvent.
func NewEnqueueRequestForServiceEvent(k8sClient client.Client, eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForServiceEvent {
	return &enqueueRequestsForServiceEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForServiceEvent)(nil)

type enqueueRequestsForServiceEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForServiceEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Service))
}

func (h *enqueueRequestsForServiceEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSvc := e.ObjectOld.(*corev1.Service)
	newSvc := e.ObjectNew.(*corev1.Service)

	// we only care below update event:
	//	1. Service annotation updates
	//	2. Service spec updates
	//	3. Service deletions
	if !equality.Semantic.DeepEqual(oldSvc.ResourceVersion, newSvc.ResourceVersion) {
		if equality.Semantic.DeepEqual(oldSvc.Annotations, newSvc.Annotations) &&
			equality.Semantic.DeepEqual(oldSvc.Spec, newSvc.Spec) &&
			equality.Semantic.DeepEqual(oldSvc.DeletionTimestamp.IsZero(), newSvc.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueImpactedIngresses(ctx, queue, newSvc)
}

func (h *enqueueRequestsForServiceEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Service))
}

func (h *enqueueRequestsForServiceEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForServiceEvent) enqueueImpactedIngresses(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], svc *corev1.Service) {
	ingList := &networkingv1.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.InNamespace(svc.GetNamespace()),
		client.MatchingFields{ingress.IndexKeyServiceRefName: svc.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	// enqueue all impacted ingresses if supported
	for index := range ingList.Items {
		ing := &ingList.Items[index]

		if !h.ingressUtils.IsIngressPendingFinalization(ing) && !h.ingressUtils.IsIngressSupported(ing) {
			continue
		}

		h.logger.V(1).Info("enqueue ingress for service event",
			"ingress", types.NamespacedName{
				Namespace: ing.Namespace,
				Name:      ing.Name,
			}.String(),
			"service", types.NamespacedName{
				Namespace: svc.Namespace,
				Name:      svc.Name,
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
