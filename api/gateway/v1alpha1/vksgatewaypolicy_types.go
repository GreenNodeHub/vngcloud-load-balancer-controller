package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vksgwpolicy,categories=gateway-api
// +kubebuilder:metadata:labels=gateway.networking.k8s.io/policy=direct
// +kubebuilder:printcolumn:name="Targets",type=string,JSONPath=`.spec.targetRefs[*].name`
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type VKSGatewayPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VKSGatewayPolicySpec   `json:"spec,omitempty"`
	Status VKSGatewayPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VKSGatewayPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VKSGatewayPolicy `json:"items"`
}

type VKSGatewayPolicySpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	TargetRefs []LocalPolicyTargetReferenceWithSectionName `json:"targetRefs"`

	SSLPolicy         *string           `json:"sslPolicy,omitempty"`
	ALPNPolicy        *string           `json:"alpnPolicy,omitempty"`
	AllowedCIDRs      []string          `json:"allowedCidrs,omitempty"`
	InsertHeaders     map[string]string `json:"insertHeaders,omitempty"`
	TimeoutClient     *metav1.Duration  `json:"timeoutClient,omitempty"`
	TimeoutMember     *metav1.Duration  `json:"timeoutMember,omitempty"`
	TimeoutConnection *metav1.Duration  `json:"timeoutConnection,omitempty"`

	CertificateIDs      []string `json:"certificateIds,omitempty"`
	ClientCertificateID *string  `json:"clientCertificateId,omitempty"`

	LoadBalancerSpec *VKSLoadBalancerSpec `json:"loadBalancerSpec,omitempty"`
}

type VKSLoadBalancerSpec struct {
	// +kubebuilder:validation:Enum=Internet;Internal;InterVPC
	Scheme         *string           `json:"scheme,omitempty"`
	PackageID      *string           `json:"packageId,omitempty"`
	SubnetID       *string           `json:"subnetId,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	LoadBalancerID *string           `json:"loadBalancerId,omitempty"`
}

type VKSGatewayPolicyStatus struct {
	CommonStatus       `json:",inline"`
	CommonPolicyStatus `json:",inline"`
}

func init() {
	SchemeBuilder.Register(&VKSGatewayPolicy{}, &VKSGatewayPolicyList{})
}
