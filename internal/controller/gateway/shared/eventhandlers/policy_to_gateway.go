package eventhandlers

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vksv1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
)

func VKSGatewayPolicyToGateway() handler.EventHandler {
	enq := func(ctx context.Context, obj client.Object, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		p, ok := obj.(*vksv1.VKSGatewayPolicy)
		if !ok {
			return
		}
		for _, t := range p.Spec.TargetRefs {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: p.Namespace, Name: string(t.Name)}})
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enq(ctx, e.Object, q)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enq(ctx, e.ObjectNew, q)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enq(ctx, e.Object, q)
		},
	}
}
