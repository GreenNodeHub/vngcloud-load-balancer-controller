package shared

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	vksv1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
)

const (
	IndexHTTPRouteByService            = "spec.rules.backendRefs.name"
	IndexHTTPRouteByParentGateway      = "spec.parentRefs.gateway"
	IndexVKSGatewayPolicyByGateway     = "spec.targetRefs.gateway"
	IndexVKSBackendPolicyByService     = "spec.targetRefs.service"
	IndexVKSHealthCheckPolicyByService = "spec.targetRefs.service"
	IndexVKSRoutePolicyByRoute         = "spec.targetRefs.httproute"
)

func RegisterIndexes(ctx context.Context, mgr manager.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwv1.HTTPRoute{}, IndexHTTPRouteByService, indexHTTPRouteByService); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwv1.HTTPRoute{}, IndexHTTPRouteByParentGateway, indexHTTPRouteByParent); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &vksv1.VKSGatewayPolicy{}, IndexVKSGatewayPolicyByGateway, indexVKSGatewayPolicyByGateway); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &vksv1.VKSBackendPolicy{}, IndexVKSBackendPolicyByService, indexVKSBackendPolicyByService); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &vksv1.VKSHealthCheckPolicy{}, IndexVKSHealthCheckPolicyByService, indexVKSHealthCheckPolicyByService); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(ctx, &vksv1.VKSRoutePolicy{}, IndexVKSRoutePolicyByRoute, indexVKSRoutePolicyByRoute); err != nil {
		return err
	}
	return nil
}

func indexHTTPRouteByService(obj client.Object) []string {
	r := obj.(*gwv1.HTTPRoute)
	var keys []string
	for _, rule := range r.Spec.Rules {
		for _, br := range rule.BackendRefs {
			ns := r.Namespace
			if br.Namespace != nil {
				ns = string(*br.Namespace)
			}
			keys = append(keys, ns+"/"+string(br.Name))
		}
	}
	return keys
}

func indexHTTPRouteByParent(obj client.Object) []string {
	r := obj.(*gwv1.HTTPRoute)
	var keys []string
	for _, p := range r.Spec.ParentRefs {
		ns := r.Namespace
		if p.Namespace != nil {
			ns = string(*p.Namespace)
		}
		if p.Kind == nil || *p.Kind == "Gateway" {
			keys = append(keys, ns+"/"+string(p.Name))
		}
	}
	return keys
}

func indexVKSGatewayPolicyByGateway(obj client.Object) []string {
	p := obj.(*vksv1.VKSGatewayPolicy)
	var keys []string
	for _, t := range p.Spec.TargetRefs {
		keys = append(keys, p.Namespace+"/"+string(t.Name))
	}
	return keys
}

func indexVKSBackendPolicyByService(obj client.Object) []string {
	p := obj.(*vksv1.VKSBackendPolicy)
	var keys []string
	for _, t := range p.Spec.TargetRefs {
		keys = append(keys, p.Namespace+"/"+string(t.Name))
	}
	return keys
}

func indexVKSHealthCheckPolicyByService(obj client.Object) []string {
	p := obj.(*vksv1.VKSHealthCheckPolicy)
	var keys []string
	for _, t := range p.Spec.TargetRefs {
		keys = append(keys, p.Namespace+"/"+string(t.Name))
	}
	return keys
}

func indexVKSRoutePolicyByRoute(obj client.Object) []string {
	p := obj.(*vksv1.VKSRoutePolicy)
	var keys []string
	for _, t := range p.Spec.TargetRefs {
		keys = append(keys, p.Namespace+"/"+string(t.Name))
	}
	return keys
}
