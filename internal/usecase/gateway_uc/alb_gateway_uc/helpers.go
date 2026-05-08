package alb_gateway_uc

import (
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
)

// durationLike is a tiny wrapper so callers can express "is this set?" without
// re-deciding nil-handling at every call site. We wrap *metav1.Duration in
// build_listeners.go via wrapDur.
type durationLike struct {
	Duration time.Duration
}

func wrapDur(d *metav1.Duration) *durationLike {
	if d == nil {
		return nil
	}
	return &durationLike{Duration: d.Duration}
}

// firstNonNilDuration walks policies in priority order and returns the first
// duration that any policy supplies. Nil policies in the slice are skipped.
func firstNonNilDuration(policies []*gwv1alpha1.VKSGatewayPolicy, get func(*gwv1alpha1.VKSGatewayPolicy) *durationLike) *durationLike {
	for _, p := range policies {
		if p == nil {
			continue
		}
		if v := get(p); v != nil {
			return v
		}
	}
	return nil
}

func firstNonNilString(policies []*gwv1alpha1.VKSGatewayPolicy, get func(*gwv1alpha1.VKSGatewayPolicy) *string) *string {
	for _, p := range policies {
		if p == nil {
			continue
		}
		if v := get(p); v != nil {
			return v
		}
	}
	return nil
}

func firstNonEmptyStringSlice(policies []*gwv1alpha1.VKSGatewayPolicy, get func(*gwv1alpha1.VKSGatewayPolicy) []string) []string {
	for _, p := range policies {
		if p == nil {
			continue
		}
		if v := get(p); len(v) > 0 {
			return v
		}
	}
	return nil
}

func firstNonEmptyStringMap(policies []*gwv1alpha1.VKSGatewayPolicy, get func(*gwv1alpha1.VKSGatewayPolicy) map[string]string) map[string]string {
	for _, p := range policies {
		if p == nil {
			continue
		}
		if v := get(p); len(v) > 0 {
			return v
		}
	}
	return nil
}

func ptr32(v int32) *int32 { return &v }

func sortStrings(s []string) { sort.Strings(s) }
