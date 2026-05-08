package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vksroutepolicy,categories=gateway-api
// +kubebuilder:metadata:labels=gateway.networking.k8s.io/policy=direct
// +kubebuilder:printcolumn:name="Targets",type=string,JSONPath=`.spec.targetRefs[*].name`
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type VKSRoutePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VKSRoutePolicySpec   `json:"spec,omitempty"`
	Status VKSRoutePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VKSRoutePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VKSRoutePolicy `json:"items"`
}

type VKSRoutePolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReferenceWithSectionName `json:"targetRefs"`

	AdditionalMatches []VKSAdditionalMatch `json:"additionalMatches,omitempty"`
	Actions           []VKSRuleAction      `json:"actions,omitempty"`
	Position          *int32               `json:"position,omitempty"`
}

type VKSAdditionalMatch struct {
	// +kubebuilder:validation:Enum=Header;QueryParam;Method;SourceIP
	Type string  `json:"type"`
	Name *string `json:"name,omitempty"`
	// +kubebuilder:validation:Enum=EQUAL_TO;STARTS_WITH;ENDS_WITH;CONTAINS;REGEX
	Compare string `json:"compare"`
	Value   string `json:"value"`
}

type VKSRuleAction struct {
	// +kubebuilder:validation:Enum=FixedResponse;Reject;Redirect
	Type          string                  `json:"type"`
	FixedResponse *VKSFixedResponseAction `json:"fixedResponse,omitempty"`
	Redirect      *VKSRedirectAction      `json:"redirect,omitempty"`
}

type VKSFixedResponseAction struct {
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=599
	StatusCode  int32   `json:"statusCode"`
	ContentType *string `json:"contentType,omitempty"`
	Body        *string `json:"body,omitempty"`
}

type VKSRedirectAction struct {
	URL             string `json:"url"`
	HTTPCode        *int32 `json:"httpCode,omitempty"`
	KeepQueryString *bool  `json:"keepQueryString,omitempty"`
}

type VKSRoutePolicyStatus struct {
	CommonStatus       `json:",inline"`
	CommonPolicyStatus `json:",inline"`
}

func init() {
	SchemeBuilder.Register(&VKSRoutePolicy{}, &VKSRoutePolicyList{})
}
