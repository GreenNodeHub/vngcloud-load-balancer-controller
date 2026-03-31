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
	"slices"

	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Condition types for GlobalLoadBalancerConfig
const (
	// GLBCConditionTypeReady indicates the GlobalLoadBalancerConfig is ready and fully reconciled
	GLBCConditionTypeReady = "Ready"
)

// Condition reasons for GlobalLoadBalancerConfig
const (
	GLBCReasonReconcileSuccess = "ReconcileSuccess"
	GLBCReasonReconcileFailed  = "ReconcileFailed"
)

// GlobalLoadBalancerConfigSpec defines the desired state of GlobalLoadBalancerConfig
type GlobalLoadBalancerConfigSpec struct {
	// should put cluster in in annotation, because it not belong to load balancer directly
	// // ClusterId is the ID of the cluster where the load balancer will be deployed. It helps in organizing resources.
	// // +optional
	// ClusterId *string `json:"clusterId,omitempty"`

	// General fields for all load balancers
	// Name is the name of the load balancer
	// +required
	Name string `json:"name,omitempty"`

	// Description is the description of the load balancer
	// +optional
	Description *string `json:"description,omitempty"`

	// Type defines the type of global load balancer
	// +required
	Type global.GlobalLoadBalancerType `json:"type"`

	// PackageId is the size/package of the load balancer
	// +optional
	PackageId *string `json:"packageId,omitempty"`

	// PaymentFlow defines the payment flow for the load balancer
	// +optional
	PaymentFlow *global.GlobalLoadBalancerPaymentFlow `json:"paymentFlow,omitempty"`

	// LoadBalancerId is managed by the controller
	// +optional
	LoadBalancerId *string `json:"loadBalancerId,omitempty"`

	// Tags are key-value pairs for load balancer tagging
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// GlobalListeners defines the array of listeners for the load balancer
	// +optional
	// +listType=map
	// +listMapKey=name
	GlobalListeners []GlobalListener `json:"globalListeners,omitempty"`

	// GlobalPools defines the array of pools for the load balancer
	// +optional
	// +listType=map
	// +listMapKey=name
	GlobalPools []GlobalPool `json:"globalPools,omitempty"`
}

type GlobalListener struct {
	// Name is the name of the listener
	// +required
	Name string `json:"name,omitempty"`

	// Description is an optional description for the listener
	// +optional
	Description *string `json:"description,omitempty"`

	// Protocol is the protocol for the listener
	// +required
	Protocol global.GlobalListenerProtocol `json:"protocol"`

	// ProtocolPort is the port number for the listener
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ProtocolPort int `json:"protocolPort"`

	// TODO
	// DefaultPoolName is the name of the default pool
	// +optional
	DefaultPoolName *string `json:"defaultPoolName,omitempty"`

	// TimeoutClient is the client timeout in seconds
	// +optional
	TimeoutClient *int32 `json:"timeoutClient,omitempty"`

	// TimeoutMember is the member timeout in seconds
	// +optional
	TimeoutMember *int32 `json:"timeoutMember,omitempty"`

	// TimeoutConnection is the connection timeout in seconds
	// +optional
	TimeoutConnection *int32 `json:"timeoutConnection,omitempty"`

	// AllowedCidrs defines the allowed CIDR blocks
	// +optional
	AllowedCidrs *string `json:"allowedCidrs,omitempty"`

	// Headers defines headers to insert into requests
	// +optional
	Headers []string `json:"headers,omitempty"`
}

type GlobalPool struct {
	// Name is the name of the pool
	// +required
	Name string `json:"name"`

	// Description is an optional description for the pool
	// +optional
	Description *string `json:"description,omitempty"`

	// Protocol is the protocol for the pool
	// +required
	Protocol global.GlobalPoolProtocol `json:"protocol"`

	// Algorithm is the load balancing algorithm for the pool
	// +optional
	Algorithm *global.GlobalPoolAlgorithm `json:"algorithm,omitempty"`

	// Stickiness enables sticky sessions for the pool
	// +optional
	Stickiness *bool `json:"stickiness,omitempty"`

	// TLSEncryption enables TLS encryption for the pool
	// +optional
	TLSEncryption *bool `json:"tlsEncryption,omitempty"`

	// HealthMonitor defines the health monitor configuration for the pool
	// +optional
	HealthMonitor GlobalPoolHealthMonitor `json:"healthMonitor,omitempty"`

	// PoolMembers is the list of members in the pool
	// +optional
	// +listType=atomic
	PoolMembers []GlobalPoolMember `json:"poolMembers,omitempty"`
}

type GlobalPoolHealthMonitor struct {
	// Protocol is the protocol used for health checks
	// +required
	Protocol global.GlobalPoolHealthCheckProtocol `json:"protocol"`

	// HealthyThreshold is the number of consecutive successful checks before marking healthy
	// +optional
	HealthyThreshold *int `json:"healthyThreshold"`

	// UnhealthyThreshold is the number of consecutive failed checks before marking unhealthy
	// +optional
	UnhealthyThreshold *int `json:"unhealthyThreshold"`

	// Interval is the time in seconds between each health check
	// +optional
	Interval *int `json:"interval"`

	// Timeout is the maximum time in seconds to wait for a response
	// +optional
	Timeout *int `json:"timeout"`

	// HealthCheckMethod specifies how the health check request is made (e.g., GET, TCP)
	// +optional
	HealthCheckMethod *global.GlobalPoolHealthCheckMethod `json:"healthCheckMethod,omitempty"`

	// HttpVersion defines which HTTP version to use for HTTP-based health checks
	// +optional
	HttpVersion *global.GlobalPoolHealthCheckHttpVersion `json:"httpVersion,omitempty"`

	// HealthCheckPath is the path used for HTTP health checks
	// +optional
	HealthCheckPath *string `json:"healthCheckPath,omitempty"`

	// DomainName is the hostname sent in the HTTP Host header
	// +optional
	DomainName *string `json:"domainName,omitempty"`

	// SuccessCode specifies which HTTP codes indicate a healthy response
	// +optional
	SuccessCode *string `json:"successCode,omitempty"`
}

type GlobalPoolMember struct {
	// Name is an optional name for the pool member
	// +optional
	Name string `json:"name,omitempty"`

	// Description is an optional description for the pool member
	// +optional
	Description *string `json:"description,omitempty"`

	// TODO
	// Region is the region where the pool member is located
	// +required
	Region string `json:"region"`

	// TODO
	// TrafficDial is the traffic dial percentage for the pool member
	// +optional
	TrafficDial *int `json:"trafficDial,omitempty"`

	// TODO
	// VpcId is the ID of the VPC where the pool member resides
	// +required
	VpcId string `json:"vpcId"`

	// TODO
	// Type is the type of the pool member
	// +required
	Type global.GlobalPoolMemberType `json:"type"`

	// TODO
	// Members is the list of member
	// +optional
	// +listType=atomic
	Members []GlobalMember `json:"members,omitempty"`
}

// TODO
type GlobalMember struct {
	// Address is the IP address of the member
	// +required
	Address string `json:"address"`

	// BackupRole indicates if the member is a backup
	// +optional
	BackupRole bool `json:"backupRole,omitempty"`

	// Description is an optional description for the member
	// +optional
	Description *string `json:"description,omitempty"`

	// MonitorPort is the port used for health monitoring
	// +optional
	MonitorPort *int `json:"monitorPort,omitempty"`

	// Name is the name of the member
	// +required
	Name string `json:"name"`

	// Port is the port on which the member receives traffic
	// +required
	Port int `json:"port"`

	// SubnetID is the ID of the subnet where the member resides
	// +required
	SubnetID string `json:"subnetId"`

	// Weight is the weight of the member for load balancing
	// +optional
	Weight *int `json:"weight,omitempty"`
}

// Equal compares two GlobalMember for equality
func (a GlobalMember) Equal(b GlobalMember) bool {
	if a.Address != b.Address || a.Name != b.Name || a.Port != b.Port || a.SubnetID != b.SubnetID || a.BackupRole != b.BackupRole {
		return false
	}
	if !ptr.Equal(a.Description, b.Description) || !ptr.Equal(a.MonitorPort, b.MonitorPort) || !ptr.Equal(a.Weight, b.Weight) {
		return false
	}
	return true
}

// GlobalLoadBalancerConfigStatus defines the observed state of GlobalLoadBalancerConfig.
type GlobalLoadBalancerConfigStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// LastReconcileTime is the timestamp of the last reconciliation attempt
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// LastReconcileMessage contains a message from the last reconciliation
	// +optional
	LastReconcileMessage string `json:"lastReconcileMessage,omitempty"`

	// LoadBalancerId is the actual ID of the load balancer in VNG Cloud
	// +optional
	LoadBalancerId *string `json:"loadBalancerId,omitempty"`

	// Vips is the list of Virtual IPs assigned to the load balancer across different regions
	// +optional
	// +listType=map
	// +listMapKey=region
	Vips []GlobalLoadBalancerVIPStatus `json:"vips,omitempty"`

	// Domains is the list of DNS hostnames assigned to the load balancer
	// +optional
	// +listType=atomic
	Domains []string `json:"domains,omitempty"`

	// CreatedTags is the map of tags created on the load balancer
	// +optional
	CreatedTags map[string]string `json:"createdTags,omitempty"`

	// CreatedPools is the list of created pool IDs
	// +optional
	// +listType=map
	// +listMapKey=name
	CreatedPools []CreatedGlobalPool `json:"createdPools,omitempty"`

	// CreatedListeners is the list of created listener IDs
	// +optional
	// +listType=map
	// +listMapKey=id
	CreatedListeners []CreatedGlobalListener `json:"createdListeners,omitempty"`

	// conditions represent the current state of the GlobalLoadBalancerConfig resource.
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

// GlobalLoadBalancerVIPStatus represents a Virtual IP assigned to the global load balancer
type GlobalLoadBalancerVIPStatus struct {
	// Address is the IP address of the VIP
	// +required
	Address string `json:"address"`

	// Region is the region where the VIP is located
	// +required
	Region string `json:"region"`

	// Status is the status of the VIP (e.g., "ACTIVE", "PENDING")
	// +optional
	Status string `json:"status,omitempty"`
}

// Equal compares two GlobalLoadBalancerVIPStatus for equality
func (a GlobalLoadBalancerVIPStatus) Equal(b GlobalLoadBalancerVIPStatus) bool {
	return a.Address == b.Address && a.Region == b.Region && a.Status == b.Status
}

type CreatedGlobalListener struct {
	// Id is the ID of the created listener
	// +required
	Id string `json:"id,omitempty"`

	// Port is the port number of the created listener
	// +required
	Port int `json:"port,omitempty"`

	// Name is the name of the listener
	// +required
	Name string `json:"name,omitempty"`
}

// Equal compares two CreatedGlobalListener for equality
func (a CreatedGlobalListener) Equal(b CreatedGlobalListener) bool {
	return a.Id == b.Id && a.Port == b.Port && a.Name == b.Name
}

type CreatedGlobalPool struct {
	// Id is the ID of the created pool
	// +required
	Id string `json:"id,omitempty"`

	// Name is the name of the created pool
	// +required
	Name string `json:"name,omitempty"`

	// CreatedPoolMembers is the list of created member IDs
	// +optional
	// +listType=atomic
	CreatedPoolMembers []CreatedGlobalPoolMember `json:"createdPoolMembers,omitempty"`
}

// Equal compares two CreatedGlobalPool for equality (order-independent for members)
func (a CreatedGlobalPool) Equal(b CreatedGlobalPool) bool {
	if a.Id != b.Id || a.Name != b.Name {
		return false
	}
	if len(a.CreatedPoolMembers) != len(b.CreatedPoolMembers) {
		return false
	}
	for _, memberA := range a.CreatedPoolMembers {
		if !slices.ContainsFunc(b.CreatedPoolMembers, memberA.Equal) {
			return false
		}
	}
	return true
}

type CreatedGlobalPoolMember struct {
	// Id is the ID of the created member
	// +required
	Id string `json:"id,omitempty"`

	// Name is the name of the created member
	// +required
	Name string `json:"name,omitempty"`

	// CreatedMembers is the list of created members
	// +optional
	CreatedMembers []GlobalMember `json:"createdMembers,omitempty"`
}

// Equal compares two CreatedGlobalPoolMember for equality (order-independent for members)
func (a CreatedGlobalPoolMember) Equal(b CreatedGlobalPoolMember) bool {
	if a.Id != b.Id || a.Name != b.Name {
		return false
	}
	if len(a.CreatedMembers) != len(b.CreatedMembers) {
		return false
	}
	for _, memberA := range a.CreatedMembers {
		if !slices.ContainsFunc(b.CreatedMembers, memberA.Equal) {
			return false
		}
	}
	return true
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=glbc
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="LoadBalancer-Id",type="string",JSONPath=".status.loadBalancerId"
// +kubebuilder:printcolumn:name="Address",type="string",JSONPath=".status.domains[0]"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// GlobalLoadBalancerConfig is the Schema for the globalloadbalancerconfigs API
type GlobalLoadBalancerConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of GlobalLoadBalancerConfig
	// +required
	Spec GlobalLoadBalancerConfigSpec `json:"spec"`

	// status defines the observed state of GlobalLoadBalancerConfig
	// +optional
	Status GlobalLoadBalancerConfigStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// GlobalLoadBalancerConfigList contains a list of GlobalLoadBalancerConfig
type GlobalLoadBalancerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalLoadBalancerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalLoadBalancerConfig{}, &GlobalLoadBalancerConfigList{})
}
