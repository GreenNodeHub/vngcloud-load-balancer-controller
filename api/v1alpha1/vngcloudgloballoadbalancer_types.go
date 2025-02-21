/*
Copyright 2024 annd2.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VngcloudGlobalLoadBalancerSpec defines the desired state of VngcloudGlobalLoadBalancer.
type VngcloudGlobalLoadBalancerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of VngcloudGlobalLoadBalancer. Edit vngcloudgloballoadbalancer_types.go to remove/update
	// Foo string `json:"foo,omitempty"`
}

// VngcloudGlobalLoadBalancerStatus defines the observed state of VngcloudGlobalLoadBalancer.
type VngcloudGlobalLoadBalancerStatus struct {
	Address string `json:"address,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vglb

// VngcloudGlobalLoadBalancer is the Schema for the vngcloudgloballoadbalancers API.
type VngcloudGlobalLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VngcloudGlobalLoadBalancerSpec   `json:"spec,omitempty"`
	Status VngcloudGlobalLoadBalancerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VngcloudGlobalLoadBalancerList contains a list of VngcloudGlobalLoadBalancer.
// +kubebuilder:printcolumn:name="FLEET ID",type=string,JSONPath=`.metadata.labels.fleet\.vngcloud\.vn\/fleet-id`
// +kubebuilder:printcolumn:name="GLB ID",type=string,JSONPath=`.metadata.annotations.vks\.vngcloud\.vn\/load-balancer-id`
// +kubebuilder:printcolumn:name="ADDRESS",type=string,JSONPath=`.status.address`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type VngcloudGlobalLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VngcloudGlobalLoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VngcloudGlobalLoadBalancer{}, &VngcloudGlobalLoadBalancerList{})
}
