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

// NewEnqueueRequestForSecretEvent constructs new enqueueRequestsForSecretEvent.
func NewEnqueueRequestForSecretEvent(k8sClient client.Client, eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForSecretEvent {
	return &enqueueRequestsForSecretEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForSecretEvent)(nil)

type enqueueRequestsForSecretEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForSecretEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Secret))
}

func (h *enqueueRequestsForSecretEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSec := e.ObjectOld.(*corev1.Secret)
	newSec := e.ObjectNew.(*corev1.Secret)

	if !equality.Semantic.DeepEqual(oldSec.ResourceVersion, newSec.ResourceVersion) {
		if equality.Semantic.DeepEqual(oldSec.Annotations, newSec.Annotations) &&
			equality.Semantic.DeepEqual(oldSec.Data, newSec.Data) &&
			equality.Semantic.DeepEqual(oldSec.StringData, newSec.StringData) &&
			equality.Semantic.DeepEqual(oldSec.Type, newSec.Type) &&
			equality.Semantic.DeepEqual(oldSec.DeletionTimestamp.IsZero(), newSec.DeletionTimestamp.IsZero()) {
			return
		}
	}

	h.enqueueImpactedIngresses(ctx, queue, newSec)
}

func (h *enqueueRequestsForSecretEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*corev1.Secret))
}

func (h *enqueueRequestsForSecretEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForSecretEvent) enqueueImpactedIngresses(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], sec *corev1.Secret) {
	ingList := &networkingv1.IngressList{}
	if err := h.k8sClient.List(context.Background(), ingList,
		client.InNamespace(sec.GetNamespace()),
		client.MatchingFields{ingress.IndexKeySecretRefName: sec.GetName()}); err != nil {
		h.logger.Error(err, "failed to fetch ingresses")
		return
	}

	// enqueue all impacted ingresses if supported
	for index := range ingList.Items {
		ing := &ingList.Items[index]

		if !h.ingressUtils.IsIngressPendingFinalization(ing) && !h.ingressUtils.IsIngressSupported(ing) {
			continue
		}

		h.logger.V(1).Info("enqueue ingress for secret event",
			"ingress", types.NamespacedName{
				Namespace: ing.Namespace,
				Name:      ing.Name,
			}.String(),
			"secret", types.NamespacedName{
				Namespace: sec.Namespace,
				Name:      sec.Name,
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
