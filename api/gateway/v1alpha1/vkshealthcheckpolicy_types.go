package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vkshcpolicy,categories=gateway-api
// +kubebuilder:metadata:labels=gateway.networking.k8s.io/policy=direct
// +kubebuilder:printcolumn:name="Protocol",type=string,JSONPath=`.spec.protocol`
// +kubebuilder:printcolumn:name="Targets",type=string,JSONPath=`.spec.targetRefs[*].name`
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type VKSHealthCheckPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VKSHealthCheckPolicySpec   `json:"spec,omitempty"`
	Status VKSHealthCheckPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VKSHealthCheckPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VKSHealthCheckPolicy `json:"items"`
}

type VKSHealthCheckPolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs"`

	// +kubebuilder:validation:Enum=HTTP;HTTPS;TCP
	Protocol           string           `json:"protocol"`
	Port               *int32           `json:"port,omitempty"`
	Interval           *metav1.Duration `json:"interval,omitempty"`
	Timeout            *metav1.Duration `json:"timeout,omitempty"`
	HealthyThreshold   *int32           `json:"healthyThreshold,omitempty"`
	UnhealthyThreshold *int32           `json:"unhealthyThreshold,omitempty"`

	HTTPHealthCheck *VKSHTTPHealthCheck `json:"httpHealthCheck,omitempty"`
}

type VKSHTTPHealthCheck struct {
	Path           *string           `json:"path,omitempty"`
	Host           *string           `json:"host,omitempty"`
	ExpectedCodes  []string          `json:"expectedCodes,omitempty"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
}

type VKSHealthCheckPolicyStatus struct {
	CommonStatus       `json:",inline"`
	CommonPolicyStatus `json:",inline"`
}

// Matches reports whether this policy targets the given PolicyTarget.
func (p *VKSHealthCheckPolicy) Matches(t pkggw.PolicyTarget) bool {
	for _, ref := range p.Spec.TargetRefs {
		if pkggw.TargetRefMatches(ref, p.Namespace, t) {
			return true
		}
	}
	return false
}

func init() {
	SchemeBuilder.Register(&VKSHealthCheckPolicy{}, &VKSHealthCheckPolicyList{})
}
