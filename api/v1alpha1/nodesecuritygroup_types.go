/*
Copyright 2024.

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
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeSecurityGroupRule defines a security group rule for NodeSecurityGroup
type NodeSecurityGroupRule struct {
	// Protocol is the protocol for the security group rule
	// +required
	Protocol networkv2.SecgroupRuleProtocol `json:"protocol"`

	// FromPort is the starting port for the rule
	// +required
	FromPort int32 `json:"fromPort"`

	// ToPort is the ending port for the rule
	// +required
	ToPort int32 `json:"toPort"`

	// CIDR is the CIDR block for the rule
	// +required
	CIDR string `json:"cidr"`

	// Description is an optional description for the rule
	// +optional
	Description string `json:"description,omitempty"`

	// Direction is the direction of the rule (ingress or egress)
	// +optional
	Direction networkv2.SecgroupRuleDirection `json:"direction,omitempty"`

	// EtherType is the ethertype of the rule (IPv4 or IPv6)
	// +optional
	EtherType networkv2.SecgroupRuleEtherType `json:"etherType,omitempty"`
}

// ManagedSecurityGroup defines a managed security group
type ManagedSecurityGroup struct {
	// Name is the name of the security group
	// +required
	Name string `json:"name"`

	// Description of the security group
	// +optional
	Description *string `json:"description,omitempty"`

	// Rules is the list of security group rules
	// +optional
	Rules []NodeSecurityGroupRule `json:"rules,omitempty"`
}

// NodeSecurityGroupSpec defines the desired state of NodeSecurityGroup
type NodeSecurityGroupSpec struct {
	// SelectNodeLabels is a map of labels to select nodes
	// +optional
	SelectNodeLabels map[string]string `json:"selectNodeLabels"`

	// AttachSecurityGroups is a list of existing security group IDs to attach to the nodes
	// +optional
	AttachSecurityGroups []string `json:"attachSecurityGroups,omitempty"`

	// ManagedSecurityGroup defines a security group to be created and managed
	// +optional
	ManagedSecurityGroup *ManagedSecurityGroup `json:"managedSecurityGroup,omitempty"`
}

// NodeInfo contains information about a selected node
type NodeInfo struct {
	// Name is the name of the node
	// +required
	Name string `json:"name"`

	// ServerID is the VNG Cloud server ID of the node
	// +optional
	ServerID string `json:"serverID,omitempty"`
}

// AttachedSecurityGroup contains information about an attached security group
type AttachedSecurityGroup struct {
	// ID is the security group ID
	// +required
	ID string `json:"id"`
}

// ServerSecurityGroupStatus tracks the security group attachment status for a specific server
type ServerSecurityGroupStatus struct {
	// ServerID is the VNG Cloud server ID
	// +required
	ServerID string `json:"serverID"`

	// AttachedSecurityGroupIDs is the list of security group IDs successfully attached to this server
	// +optional
	AttachedSecurityGroupIDs []string `json:"attachedSecurityGroupIDs,omitempty"`

	// Error contains the error message if attachment failed
	// +optional
	Error *string `json:"error,omitempty"`

	// // LastUpdateTime is the timestamp of the last update
	// // +optional
	// LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

type ManagedSecurityGroupStatus struct {
	// Id is the id of the managed security group
	// +optional
	Id *string `json:"id,omitempty"`

	// Error contains the error message when ensuring the managed security group
	// +optional
	Error *string `json:"error,omitempty"`
}

// NodeSecurityGroupStatus defines the observed state of NodeSecurityGroup.
type NodeSecurityGroupStatus struct {
	// SelectedNodes is the list of nodes that match the node selector
	// +optional
	SelectedNodes []NodeInfo `json:"selectedNodes,omitempty"`

	// ManagedSecurityGroup contains the status of the managed security group
	// +optional
	ManagedSecurityGroup ManagedSecurityGroupStatus `json:"managedSecurityGroup,omitempty"`

	// ServerSecurityGroups tracks the security group attachment status for each server
	// +optional
	// +listType=map
	// +listMapKey=serverID
	ServerSecurityGroups []ServerSecurityGroupStatus `json:"serverSecurityGroups,omitempty"`

	// conditions represent the current state of the NodeSecurityGroup resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=nsg

// NodeSecurityGroup is the Schema for the nodesecuritygroups API
type NodeSecurityGroup struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of NodeSecurityGroup
	// +required
	Spec NodeSecurityGroupSpec `json:"spec"`

	// status defines the observed state of NodeSecurityGroup
	// +optional
	Status NodeSecurityGroupStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// NodeSecurityGroupList contains a list of NodeSecurityGroup
type NodeSecurityGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSecurityGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeSecurityGroup{}, &NodeSecurityGroupList{})
}
