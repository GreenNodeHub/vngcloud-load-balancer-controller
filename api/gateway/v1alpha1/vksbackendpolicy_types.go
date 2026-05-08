package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vksbpolicy,categories=gateway-api
// +kubebuilder:metadata:labels=gateway.networking.k8s.io/policy=direct
// +kubebuilder:printcolumn:name="Targets",type=string,JSONPath=`.spec.targetRefs[*].name`
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type VKSBackendPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VKSBackendPolicySpec   `json:"spec,omitempty"`
	Status VKSBackendPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VKSBackendPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VKSBackendPolicy `json:"items"`
}

type VKSBackendPolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReference `json:"targetRefs"`

	// +kubebuilder:validation:Enum=ROUND_ROBIN;LEAST_CONNECTIONS;SOURCE_IP
	PoolAlgorithm *string `json:"poolAlgorithm,omitempty"`

	// Stickiness enables sticky sessions on the pool. The vngcloud LB API
	// exposes only an on/off flag; cookie name and TTL are not configurable.
	Stickiness *bool `json:"stickiness,omitempty"`

	EnableTLSEncryption *bool             `json:"enableTLSEncryption,omitempty"`
	TargetNodeLabels    map[string]string `json:"targetNodeLabels,omitempty"`
	ManageDFPMembers    *bool             `json:"manageDFPMembers,omitempty"`
}

type VKSBackendPolicyStatus struct {
	CommonStatus       `json:",inline"`
	CommonPolicyStatus `json:",inline"`
}

// Matches reports whether this policy targets the given PolicyTarget.
func (p *VKSBackendPolicy) Matches(t pkggw.PolicyTarget) bool {
	for _, ref := range p.Spec.TargetRefs {
		if pkggw.TargetRefMatches(ref, p.Namespace, t) {
			return true
		}
	}
	return false
}

func init() {
	SchemeBuilder.Register(&VKSBackendPolicy{}, &VKSBackendPolicyList{})
}
