package alb_gateway_uc

import (
	"context"
	"fmt"

	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	sharedUC "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
	pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// applyBackendPolicyToPool merges fields from a VKSBackendPolicy targeting
// the named Service into pool. The policy is resolved oldest-wins; conflict
// reporting is the validator controller's job.
//
// vngcloud Pool fields touched: Algorithm, Stickiness, TLSEncryption.
// VKSBackendPolicy.TargetNodeLabels and ManageDFPMembers are controller-side
// (not in LBC.Pool); they're handled when we add target-mode selection in
// a future phase.
func (t *defaultGatewayBuildTask) applyBackendPolicyToPool(ctx context.Context, pool *v1alpha1.Pool, ns, svcName string) error {
	bp, err := t.resolveBackendPolicy(ctx, ns, svcName)
	if err != nil {
		return err
	}
	if bp == nil {
		return nil
	}
	s := bp.Spec
	if s.PoolAlgorithm != nil {
		alg := v2.PoolAlgorithm(*s.PoolAlgorithm)
		pool.Algorithm = &alg
	}
	if s.Stickiness != nil {
		v := *s.Stickiness
		pool.Stickiness = &v
	}
	if s.EnableTLSEncryption != nil {
		v := *s.EnableTLSEncryption
		pool.TLSEncryption = &v
	}
	return nil
}

// applyHealthCheckPolicyToPool merges VKSHealthCheckPolicy fields into the
// pool's health monitor. Conflict resolution is oldest-wins. When two
// backendRefs in the same rule have conflicting policies, the rule fails
// translation; that's enforced one level up.
func (t *defaultGatewayBuildTask) applyHealthCheckPolicyToPool(ctx context.Context, pool *v1alpha1.Pool, ns, svcName string) error {
	hp, err := t.resolveHealthCheckPolicy(ctx, ns, svcName)
	if err != nil {
		return err
	}
	if hp == nil {
		return nil
	}
	s := hp.Spec
	mon := v1alpha1.PoolHealthMonitor{
		Protocol: v2.HealthCheckProtocol(s.Protocol),
	}
	if s.Interval != nil {
		mon.Interval = ptrInt(int(s.Interval.Duration.Seconds()))
	}
	if s.Timeout != nil {
		mon.Timeout = ptrInt(int(s.Timeout.Duration.Seconds()))
	}
	if s.HealthyThreshold != nil {
		mon.HealthyThreshold = ptrInt(int(*s.HealthyThreshold))
	}
	if s.UnhealthyThreshold != nil {
		mon.UnhealthyThreshold = ptrInt(int(*s.UnhealthyThreshold))
	}
	if s.HTTPHealthCheck != nil {
		mon.HealthCheckPath = s.HTTPHealthCheck.Path
		mon.DomainName = s.HTTPHealthCheck.Host
		if len(s.HTTPHealthCheck.ExpectedCodes) > 0 {
			// LBC stores SuccessCode as a single string. Join with commas to
			// preserve any multi-value expression the user wrote (e.g. "200-299,301").
			joined := joinExpectedCodes(s.HTTPHealthCheck.ExpectedCodes)
			mon.SuccessCode = &joined
		}
	}
	pool.HealthMonitor = mon
	return nil
}

func (t *defaultGatewayBuildTask) resolveBackendPolicy(ctx context.Context, ns, svcName string) (*gwv1alpha1.VKSBackendPolicy, error) {
	var list gwv1alpha1.VKSBackendPolicyList
	if err := t.uc.k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list VKSBackendPolicy in %s: %w", ns, err)
	}
	cands := make([]*gwv1alpha1.VKSBackendPolicy, 0, len(list.Items))
	for i := range list.Items {
		cands = append(cands, &list.Items[i])
	}
	target := pkggw.PolicyTarget{Group: "", Kind: "Service", Namespace: ns, Name: svcName}
	win, _ := sharedUC.ResolveDirectPolicy(cands, target)
	return win, nil
}

func (t *defaultGatewayBuildTask) resolveHealthCheckPolicy(ctx context.Context, ns, svcName string) (*gwv1alpha1.VKSHealthCheckPolicy, error) {
	var list gwv1alpha1.VKSHealthCheckPolicyList
	if err := t.uc.k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("list VKSHealthCheckPolicy in %s: %w", ns, err)
	}
	cands := make([]*gwv1alpha1.VKSHealthCheckPolicy, 0, len(list.Items))
	for i := range list.Items {
		cands = append(cands, &list.Items[i])
	}
	target := pkggw.PolicyTarget{Group: "", Kind: "Service", Namespace: ns, Name: svcName}
	win, _ := sharedUC.ResolveDirectPolicy(cands, target)
	return win, nil
}

func ptrInt(v int) *int { return &v }

func joinExpectedCodes(codes []string) string {
	out := ""
	for i, c := range codes {
		if i > 0 {
			out += ","
		}
		out += c
	}
	return out
}
