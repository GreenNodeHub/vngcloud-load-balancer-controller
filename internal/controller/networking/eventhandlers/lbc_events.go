package eventhandlers

import (
	"context"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/ingress"
)

// NewEnqueueRequestForLbcEvent constructs new enqueueRequestsForLbcEvent.
func NewEnqueueRequestForLbcEvent(k8sClient client.Client, eventRecorder record.EventRecorder,
	ingressUtils ingress.IngressUtils, logger logr.Logger) *enqueueRequestsForLbcEvent {
	return &enqueueRequestsForLbcEvent{
		k8sClient:     k8sClient,
		eventRecorder: eventRecorder,
		ingressUtils:  ingressUtils,
		logger:        logger,
	}
}

var _ handler.EventHandler = (*enqueueRequestsForLbcEvent)(nil)

type enqueueRequestsForLbcEvent struct {
	k8sClient     client.Client
	eventRecorder record.EventRecorder
	ingressUtils  ingress.IngressUtils
	logger        logr.Logger
}

func (h *enqueueRequestsForLbcEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*v1alpha1.LoadBalancerConfig))
}

func (h *enqueueRequestsForLbcEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldLbc := e.ObjectOld.(*v1alpha1.LoadBalancerConfig)
	newLbc := e.ObjectNew.(*v1alpha1.LoadBalancerConfig)

	// Skip reconciliation if only unimportant fields changed
	if equality.Semantic.DeepEqual(oldLbc.Spec, newLbc.Spec) &&
		equality.Semantic.DeepEqual(oldLbc.Status.Address, newLbc.Status.Address) &&
		oldLbc.DeletionTimestamp.IsZero() == newLbc.DeletionTimestamp.IsZero() {
		return
	}

	h.enqueueImpactedIngresses(ctx, queue, newLbc)
}

func (h *enqueueRequestsForLbcEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.enqueueImpactedIngresses(ctx, queue, e.Object.(*v1alpha1.LoadBalancerConfig))
}

func (h *enqueueRequestsForLbcEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
}

func (h *enqueueRequestsForLbcEvent) enqueueImpactedIngresses(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request], lbcObj *v1alpha1.LoadBalancerConfig) {
	// check label to know kind of resource and name
	objLabel := lbcObj.GetLabels()
	if objLabel == nil {
		return
	}
	kind := lbcObj.Labels[domain.LabelOwnerResourceKind]
	if kind != "Ingress" {
		return
	}
	name := lbcObj.Labels[domain.LabelOwnerResourceName]

	ing := &networkingv1.Ingress{}
	if err := h.k8sClient.Get(ctx, types.NamespacedName{
		Namespace: lbcObj.Namespace,
		Name:      name,
	}, ing); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return
		}
		h.logger.Error(err, "failed to fetch ingress")
		return
	}

	if lbcObj.Labels[domain.LabelOwnerResourceUid] != string(ing.UID) {
		return
	}

	if !h.ingressUtils.IsIngressPendingFinalization(ing) && !h.ingressUtils.IsIngressSupported(ing) {
		return
	}

	h.logger.V(1).Info("Enqueue Ingress", "namespace", ing.Namespace, "name", ing.Name)
	queue.Add(reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ing.Namespace,
			Name:      ing.Name,
		},
	})
}
