package gateway

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type fakeObj struct {
	name, ns, gvk string
}

func TestTargetRefMatches(t *testing.T) {
	grp := gwv1alpha2.Group("gateway.networking.k8s.io")
	tref := gwv1alpha2.LocalPolicyTargetReference{
		Group: grp,
		Kind:  gwv1alpha2.Kind("Gateway"),
		Name:  gwv1alpha2.ObjectName("prod-gw"),
	}
	ok := TargetRefMatches(tref, "ns", PolicyTarget{
		Group:     "gateway.networking.k8s.io",
		Kind:      "Gateway",
		Name:      "prod-gw",
		Namespace: "ns",
	})
	if !ok {
		t.Fatal("expected match")
	}
}

func TestTargetRefMatches_DifferentName(t *testing.T) {
	tref := gwv1alpha2.LocalPolicyTargetReference{
		Group: "gateway.networking.k8s.io",
		Kind:  "Gateway",
		Name:  "prod-gw",
	}
	if TargetRefMatches(tref, "ns", PolicyTarget{
		Group: "gateway.networking.k8s.io", Kind: "Gateway",
		Name: "staging-gw", Namespace: "ns",
	}) {
		t.Fatal("unexpected match")
	}
	_ = metav1.Now()
}
