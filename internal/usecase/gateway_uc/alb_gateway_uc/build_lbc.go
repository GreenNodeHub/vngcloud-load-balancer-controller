package alb_gateway_uc

import (
	"context"
	"fmt"
	"reflect"

	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/common"
	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	gwv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	sharedUC "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
	pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// defaultGatewayBuildTask carries per-reconcile state and is the rough analogue
// of ingress_uc's defaultModelBuildTask. One task per Gateway reconcile.
type defaultGatewayBuildTask struct {
	uc     *albGatewayUseCase
	logger *logrus.Entry
	gw     *gwv1.Gateway

	// Resolved at translation time.
	unscopedPolicy   *gwv1alpha1.VKSGatewayPolicy             // for LB-level fields and listener defaults
	listenerPolicies map[string]*gwv1alpha1.VKSGatewayPolicy  // by Gateway listener name
}

// run is the per-reconcile entry. Phase A only translates LB-level fields;
// listeners, certs, pools, and policies land in subsequent phases and will
// extend buildLoadBalancerConfig in place.
func (t *defaultGatewayBuildTask) run(ctx context.Context) error {
	if err := t.resolveGatewayPolicies(ctx); err != nil {
		return err
	}
	return t.buildLoadBalancerConfig(ctx)
}

// resolveGatewayPolicies finds the unscoped policy (LB-level fields + listener
// defaults) and per-listener policies (by sectionName). Conflict losers are
// stamped onto status by the policy validator controllers, not here.
func (t *defaultGatewayBuildTask) resolveGatewayPolicies(ctx context.Context) error {
	var list gwv1alpha1.VKSGatewayPolicyList
	if err := t.uc.k8sClient.List(ctx, &list, client.InNamespace(t.gw.Namespace)); err != nil {
		return fmt.Errorf("list VKSGatewayPolicy: %w", err)
	}
	cands := make([]*gwv1alpha1.VKSGatewayPolicy, 0, len(list.Items))
	for i := range list.Items {
		cands = append(cands, &list.Items[i])
	}

	unscopedTarget := pkggw.PolicyTarget{
		Group: "gateway.networking.k8s.io", Kind: "Gateway",
		Namespace: t.gw.Namespace, Name: t.gw.Name,
	}
	t.unscopedPolicy, _ = sharedUC.ResolveDirectPolicy(cands, unscopedTarget)

	t.listenerPolicies = make(map[string]*gwv1alpha1.VKSGatewayPolicy, len(t.gw.Spec.Listeners))
	for _, l := range t.gw.Spec.Listeners {
		listenerTarget := pkggw.PolicyTarget{
			Group: "gateway.networking.k8s.io", Kind: "Gateway",
			Namespace:   t.gw.Namespace,
			Name:        t.gw.Name,
			SectionName: string(l.Name),
		}
		win, _ := sharedUC.ResolveDirectPolicy(cands, listenerTarget)
		if win != nil {
			t.listenerPolicies[string(l.Name)] = win
		}
	}
	return nil
}

// buildLoadBalancerConfig finds (or creates) the LBC owned by this Gateway and
// rewrites only the fields the Gateway controller is the source of truth for.
// Mirrors ingress_uc's pattern: read existing, mutate Spec, patch only on
// real change to avoid generation churn.
func (t *defaultGatewayBuildTask) buildLoadBalancerConfig(ctx context.Context) error {
	lbcs, err := t.uc.listOwnedLBCs(ctx, t.gw)
	if err != nil {
		return err
	}
	if len(lbcs) > 1 {
		return fmt.Errorf("multiple LBCs found for Gateway %s/%s; cannot reconcile", t.gw.Namespace, t.gw.Name)
	}

	var (
		lbc       *v1alpha1.LoadBalancerConfig
		oldLBC    *v1alpha1.LoadBalancerConfig
		isCreated bool
	)
	if len(lbcs) == 1 {
		lbc = lbcs[0]
		oldLBC = lbc.DeepCopy()
		isCreated = true
	} else {
		lbc = &v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    t.gw.Namespace,
				GenerateName: t.gw.Name + "-",
			},
			Spec: v1alpha1.LoadBalancerConfigSpec{},
		}
	}

	// Owner labels — the same {kind, name, uid} set Ingress/Service use, so
	// kubectl describe and existing reverse-lookups all keep working.
	if lbc.Labels == nil {
		lbc.Labels = make(map[string]string)
	}
	lbc.Labels[domain.LabelOwnerResourceKind] = domain.OwnerKindGateway
	lbc.Labels[domain.LabelOwnerResourceName] = t.gw.Name
	lbc.Labels[domain.LabelOwnerResourceUid] = string(t.gw.UID)
	lbc.Labels[domain.OwnerLabelGatewayUID] = string(t.gw.UID)

	// Resolve subnet/zone/network. Phase A only honors what the unscoped
	// VKSGatewayPolicy.LoadBalancerSpec.SubnetID supplies and the cluster
	// defaults; richer subnet/zone resolution (e.g. inferring from a Gateway
	// address selector) is Phase 2 territory.
	subnetID, networkID, zone, _ := t.resolveSubnetAndZone()
	if subnetID == "" || networkID == "" || zone == "" {
		return fmt.Errorf("could not resolve default subnet/zone for Gateway %s/%s; set VKSGatewayPolicy.LoadBalancerSpec.SubnetID or wait for cluster default network discovery",
			t.gw.Namespace, t.gw.Name)
	}

	// Type, SubnetId, ZoneId, LoadBalancerName are mirrored from the cloud LB
	// by the LBC controller. Writing them on every reconcile fights that sync
	// and produces an infinite reconcile loop (see commit 1bbc823). Set them
	// only at create time.
	lbc.Spec.VpcId = networkID
	if !isCreated {
		lbc.Spec.Type = v2.LoadBalancerTypeLayer7
		lbc.Spec.SubnetId = subnetID
		lbc.Spec.ZoneId = common.Zone(zone)
		lbc.Spec.LoadBalancerName = t.buildLoadBalancerName()
	}

	if t.uc.clusterId != "" {
		lbc.Spec.ClusterId = &t.uc.clusterId
	}

	// LB-level fields from VKSGatewayPolicy.LoadBalancerSpec (only the
	// unscoped policy contributes here per the design spec).
	t.applyLoadBalancerSpec(lbc)

	listeners, err := t.buildListeners()
	if err != nil {
		return err
	}
	lbc.Spec.CreateCertificates = t.buildCreateCertificates()

	pools, listenerPolicies, err := t.buildPoolsAndPolicies(ctx)
	if err != nil {
		return err
	}
	lbc.Spec.Pools = pools

	// Fold per-listener policies onto the matching listener entry.
	for i := range listeners {
		if pol, ok := listenerPolicies[listeners[i].Name]; ok {
			listeners[i].Policies = pol
		}
	}
	lbc.Spec.Listeners = listeners

	if !isCreated {
		if err := t.uc.k8sRepo.CreateLoadBalancerConfig(ctx, lbc); err != nil {
			return fmt.Errorf("create LBC for Gateway %s/%s: %w", t.gw.Namespace, t.gw.Name, err)
		}
		t.logger.Infof("created LBC %s/%s", lbc.Namespace, lbc.Name)
		return nil
	}
	if reflect.DeepEqual(oldLBC.Spec, lbc.Spec) && reflect.DeepEqual(oldLBC.Labels, lbc.Labels) {
		return nil
	}
	if err := t.uc.k8sRepo.PatchLoadBalancerConfig(ctx, lbc, client.MergeFrom(oldLBC)); err != nil {
		return fmt.Errorf("patch LBC %s/%s: %w", lbc.Namespace, lbc.Name, err)
	}
	t.logger.Infof("patched LBC %s/%s", lbc.Namespace, lbc.Name)
	return nil
}

// applyLoadBalancerSpec writes the LB-level fields the unscoped
// VKSGatewayPolicy contributes. Listener-level fields (timeouts, allowedCidrs,
// insertHeaders) are applied per-listener in Phase C.
func (t *defaultGatewayBuildTask) applyLoadBalancerSpec(lbc *v1alpha1.LoadBalancerConfig) {
	if t.unscopedPolicy == nil || t.unscopedPolicy.Spec.LoadBalancerSpec == nil {
		return
	}
	s := t.unscopedPolicy.Spec.LoadBalancerSpec
	if s.Scheme != nil {
		scheme := v2.LoadBalancerScheme(*s.Scheme)
		lbc.Spec.Scheme = &scheme
	}
	if s.PackageID != nil {
		lbc.Spec.PackageId = s.PackageID
	}
	if s.LoadBalancerID != nil {
		lbc.Spec.LoadBalancerId = s.LoadBalancerID
	}
	if len(s.Tags) > 0 {
		if lbc.Spec.Tags == nil {
			lbc.Spec.Tags = make(map[string]string, len(s.Tags))
		}
		for k, v := range s.Tags {
			lbc.Spec.Tags[k] = v
		}
	}
}

// buildLoadBalancerName generates a deterministic, length-bounded LB name from
// the Gateway identity. The cloud-side limit is 50 chars (consts.DEFAULT_PORTAL_NAME_LENGTH).
func (t *defaultGatewayBuildTask) buildLoadBalancerName() string {
	uidPrefix := string(t.gw.UID)
	if len(uidPrefix) > 8 {
		uidPrefix = uidPrefix[:8]
	}
	name := fmt.Sprintf("vks-gw-%s-%s", t.gw.Name, uidPrefix)
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// resolveSubnetAndZone returns (subnetID, networkID, zone, cidr). Phase A only
// supports the path "VKSGatewayPolicy supplies subnet, controller probes node
// for network/zone." Existing-LBC adoption is intentionally not handled yet —
// the LBC controller's syncLBCSpecFromLoadBalancer mirrors those fields back.
func (t *defaultGatewayBuildTask) resolveSubnetAndZone() (string, string, string, string) {
	subnetID := t.uc.defaultSubnetId
	if t.unscopedPolicy != nil && t.unscopedPolicy.Spec.LoadBalancerSpec != nil &&
		t.unscopedPolicy.Spec.LoadBalancerSpec.SubnetID != nil {
		subnetID = *t.unscopedPolicy.Spec.LoadBalancerSpec.SubnetID
	}
	return subnetID, t.uc.defaultNetworkId, string(t.uc.defaultZone), t.uc.defaultSubnetCIDR
}

// listOwnedLBCs finds the LBC(s) owned by gw via the
// vks.vngcloud.vn/owner-resource-uid label. Caller treats >1 as an error.
func (uc *albGatewayUseCase) listOwnedLBCs(ctx context.Context, gw *gwv1.Gateway) ([]*v1alpha1.LoadBalancerConfig, error) {
	var list v1alpha1.LoadBalancerConfigList
	if err := uc.k8sRepo.ListLoadBalancerConfig(ctx, &list,
		client.InNamespace(gw.Namespace),
		client.MatchingLabels{
			domain.LabelOwnerResourceKind: domain.OwnerKindGateway,
			domain.LabelOwnerResourceUid:  string(gw.UID),
		},
	); err != nil {
		return nil, fmt.Errorf("list LBC owned by Gateway %s/%s: %w", gw.Namespace, gw.Name, err)
	}
	out := make([]*v1alpha1.LoadBalancerConfig, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}
