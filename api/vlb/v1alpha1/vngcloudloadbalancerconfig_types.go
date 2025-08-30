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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LoadBalancerType defines the type of load balancer
// +kubebuilder:validation:Enum=network;application
type LoadBalancerType string

const (
	// NetworkLoadBalancer is a Layer 4 (network) load balancer
	NetworkLoadBalancer LoadBalancerType = "network"
	// ApplicationLoadBalancer is a Layer 7 (application) load balancer
	ApplicationLoadBalancer LoadBalancerType = "application"
)

// NetworkLoadBalancerConfig contains configuration specific to network (L4) load balancers
type NetworkLoadBalancerConfig struct {
	// EnableProxyProtocol enables proxy protocol for network load balancers
	// +optional
	EnableProxyProtocol *bool `json:"enableProxyProtocol,omitempty"`
}

// HealthCheckConfig contains health check configuration
type HealthCheckConfig struct {
	// HealthcheckPort is the port for health checks
	// +optional
	HealthcheckPort *int32 `json:"healthcheckPort,omitempty"`

	// HealthcheckProtocol is the protocol for health checks
	// +optional
	// +kubebuilder:validation:Enum=TCP;HTTP;HTTPS
	HealthcheckProtocol *string `json:"healthcheckProtocol,omitempty"`

	// HealthcheckIntervalSeconds is the interval between health checks
	// +optional
	HealthcheckIntervalSeconds *int32 `json:"healthcheckIntervalSeconds,omitempty"`

	// HealthcheckTimeoutSeconds is the timeout for health checks
	// +optional
	HealthcheckTimeoutSeconds *int32 `json:"healthcheckTimeoutSeconds,omitempty"`

	// HealthyThresholdCount is the number of successful checks before considering healthy
	// +optional
	HealthyThresholdCount *int32 `json:"healthyThresholdCount,omitempty"`

	// UnhealthyThresholdCount is the number of failed checks before considering unhealthy
	// +optional
	UnhealthyThresholdCount *int32 `json:"unhealthyThresholdCount,omitempty"`

	// HTTP/HTTPS specific health check fields
	// SuccessCodes are the success codes for HTTP/HTTPS health checks
	// +optional
	SuccessCodes *string `json:"successCodes,omitempty"`

	// HealthcheckPath is the path for HTTP/HTTPS health checks
	// +optional
	HealthcheckPath *string `json:"healthcheckPath,omitempty"`

	// HealthcheckHttpMethod is the HTTP method for health checks
	// +optional
	// +kubebuilder:validation:Enum=GET;POST;PUT;DELETE;HEAD
	HealthcheckHttpMethod *string `json:"healthcheckHttpMethod,omitempty"`

	// HealthcheckHttpVersion is the HTTP version for health checks
	// +optional
	HealthcheckHttpVersion *string `json:"healthcheckHttpVersion,omitempty"`

	// HealthcheckHttpDomainName is the domain name for HTTP/HTTPS health checks
	// +optional
	HealthcheckHttpDomainName *string `json:"healthcheckHttpDomainName,omitempty"`
}

// InsertHeader defines a header to be inserted in requests
type InsertHeader struct {
	// HeaderName is the name of the header to insert
	// +required
	HeaderName string `json:"headerName"`

	// HeaderValue is the value of the header to insert
	// +required
	HeaderValue string `json:"headerValue"`
}

// L7Rule defines a Layer 7 rule for policies
type L7Rule struct {
	// CompareType is how to compare the rule value
	// +required
	// +kubebuilder:validation:Enum=CONTAINS;STARTS_WITH;ENDS_WITH;REGEX;EQUAL_TO
	CompareType string `json:"compareType"`

	// RuleValue is the value to compare against
	// +required
	RuleValue string `json:"ruleValue"`

	// RuleType is the type of rule
	// +required
	// +kubebuilder:validation:Enum=HOST_NAME;PATH;FILE_TYPE;HEADER;COOKIE
	RuleType string `json:"ruleType"`
}

// Policy defines a policy configuration for application load balancer listeners
type Policy struct {
	// Name is the name of the policy
	// +required
	Name string `json:"name"`

	// Description is an optional description for the policy
	// +optional
	Description *string `json:"description,omitempty"`

	// RedirectPoolName is the name of the pool to redirect to
	// +optional
	RedirectPoolName *string `json:"redirectPoolName,omitempty"`

	// Action defines the action to take
	// +required
	// +kubebuilder:validation:Enum=REDIRECT_TO_POOL;REDIRECT_TO_URL;REJECT
	Action string `json:"action"`

	// RedirectUrl is the URL to redirect to (for REDIRECT_TO_URL action)
	// +optional
	RedirectUrl *string `json:"redirectUrl,omitempty"`

	// RedirectHttpCode is the HTTP code to use for redirect
	// +optional
	// +kubebuilder:validation:Enum=301;302;303;307;308
	RedirectHttpCode *int32 `json:"redirectHttpCode,omitempty"`

	// KeepQueryString determines if query string should be kept on redirect
	// +optional
	KeepQueryString *bool `json:"keepQueryString,omitempty"`

	// Position is the position/priority of the policy
	// +optional
	// +kubebuilder:validation:Minimum=1
	Position int32 `json:"position,omitempty"`

	// L7Rules is the list of L7 rules for this policy
	// +optional
	L7Rules []L7Rule `json:"l7Rules,omitempty"`
}

// Pool defines a pool configuration for the load balancer
type Pool struct {
	// Name is the name of the pool
	// +required
	Name string `json:"name"`

	// Protocol is the protocol for the pool
	// +required
	// +kubebuilder:validation:Enum=TCP;UDP;HTTP;HTTPS;PROXY
	Protocol string `json:"protocol"`

	// Description is an optional description for the pool
	// +optional
	Description *string `json:"description,omitempty"`

	// LoadBalanceMethod is the load balancing method for the pool
	// +optional
	// +kubebuilder:default=ROUND_ROBIN
	// +kubebuilder:validation:Enum=ROUND_ROBIN;LEAST_CONNECTIONS;SOURCE_IP
	LoadBalanceMethod string `json:"loadBalanceMethod,omitempty"`

	// Stickiness enables sticky sessions for the pool
	// +optional
	// +kubebuilder:default=false
	Stickiness bool `json:"stickiness,omitempty"`

	// TLSEncryption enables TLS encryption for the pool
	// +optional
	// +kubebuilder:default=false
	TLSEncryption bool `json:"tlsEncryption,omitempty"`
}

// Listener defines a listener configuration for the load balancer
type Listener struct {
	// Name is the name of the listener
	// +optional
	Name *string `json:"name,omitempty"`

	// Description is an optional description for the listener
	// +optional
	Description *string `json:"description,omitempty"`

	// Protocol is the protocol for the listener
	// +required
	// +kubebuilder:validation:Enum=TCP;UDP;HTTP;HTTPS
	Protocol string `json:"protocol"`

	// ProtocolPort is the port number for the listener
	// +required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ProtocolPort int32 `json:"protocolPort"`

	// DefaultPoolName is the name of the default pool
	// +optional
	DefaultPoolName *string `json:"defaultPoolName,omitempty"`

	// TimeoutClient is the client timeout in seconds
	// +optional
	// +kubebuilder:default=50
	TimeoutClient int32 `json:"timeoutClient,omitempty"`

	// TimeoutMember is the member timeout in seconds
	// +optional
	// +kubebuilder:default=50
	TimeoutMember int32 `json:"timeoutMember,omitempty"`

	// TimeoutConnection is the connection timeout in seconds
	// +optional
	// +kubebuilder:default=5
	TimeoutConnection int32 `json:"timeoutConnection,omitempty"`

	// AllowedCidrs defines the allowed CIDR blocks
	// +optional
	// +kubebuilder:default="0.0.0.0/0"
	AllowedCidrs string `json:"allowedCidrs,omitempty"`

	// Headers is a list of headers to forward
	// +optional
	Headers []string `json:"headers,omitempty"`

	// InsertHeaders defines headers to insert into requests
	// +optional
	InsertHeaders []InsertHeader `json:"insertHeaders,omitempty"`

	// CertificateAuthorities is a list of certificate authority IDs
	// +optional
	CertificateAuthorities []string `json:"certificateAuthorities,omitempty"`

	// DefaultCertificateAuthority is the default certificate authority
	// +optional
	DefaultCertificateAuthority *string `json:"defaultCertificateAuthority,omitempty"`

	// ClientCertificateAuthentication defines client certificate authentication settings
	// +optional
	ClientCertificateAuthentication *string `json:"clientCertificateAuthentication,omitempty"`

	// Policies is the list of policies for this listener (for application load balancers)
	// +optional
	Policies []Policy `json:"policies,omitempty"`
}

// ApplicationLoadBalancerConfig contains configuration specific to application (L7) load balancers
type ApplicationLoadBalancerConfig struct {
	// EnableStickySession enables sticky sessions for application load balancers
	// +optional
	EnableStickySession *bool `json:"enableStickySession,omitempty"`

	// EnableTLSEncryption enables TLS encryption for application load balancers
	// +optional
	EnableTLSEncryption *bool `json:"enableTLSEncryption,omitempty"`

	// CertificateIDs are the SSL certificate IDs for HTTPS listeners
	// +optional
	CertificateIDs []string `json:"certificateIDs,omitempty"`

	// ClientCertificateID is the client certificate ID for mutual TLS
	// +optional
	ClientCertificateID *string `json:"clientCertificateID,omitempty"`

	// Header configuration for application load balancers
	// +optional
	Header map[string]string `json:"header,omitempty"`

	// InsertHeaders are headers to insert into requests
	// +optional
	InsertHeaders map[string]string `json:"insertHeaders,omitempty"`

	// ImplementationSpecificParams are implementation-specific parameters
	// +optional
	ImplementationSpecificParams map[string]string `json:"implementationSpecificParams,omitempty"`

	// AutoReorderPolicies enables automatic reordering of policies
	// +optional
	AutoReorderPolicies *bool `json:"autoReorderPolicies,omitempty"`
}

// VngcloudLoadBalancerConfigSpec defines the desired state of VngcloudLoadBalancerConfig
type VngcloudLoadBalancerConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Type defines the type of load balancer (network or application)
	// +required
	// +kubebuilder:validation:Enum=network;application
	Type LoadBalancerType `json:"type"`

	// General fields for all load balancers
	// LoadBalancerName is the name of the load balancer (only set via annotation)
	// +optional
	LoadBalancerName *string `json:"loadBalancerName,omitempty"`

	// PackageID is the size/package of the load balancer
	// +optional
	PackageID *string `json:"packageID,omitempty"`

	// Scheme defines if the load balancer is internal or external
	// +optional
	// +kubebuilder:validation:Enum=internal;external
	Scheme *string `json:"scheme,omitempty"`

	// TargetType defines the target type (instance or ip)
	// +optional
	// +kubebuilder:validation:Enum=instance;ip
	TargetType *string `json:"targetType,omitempty"`

	// LoadBalancerID is managed by the controller
	// +optional
	LoadBalancerID *string `json:"loadBalancerID,omitempty"`

	// Ignore tells the controller to ignore this resource
	// +optional
	Ignore *bool `json:"ignore,omitempty"`

	// IdleTimeoutClient is the idle timeout for client connections
	// +optional
	IdleTimeoutClient *int32 `json:"idleTimeoutClient,omitempty"`

	// IdleTimeoutMember is the idle timeout for member connections
	// +optional
	IdleTimeoutMember *int32 `json:"idleTimeoutMember,omitempty"`

	// IdleTimeoutConnection is the idle timeout for connections
	// +optional
	IdleTimeoutConnection *int32 `json:"idleTimeoutConnection,omitempty"`

	// InboundCIDRs defines the inbound CIDR blocks
	// +optional
	InboundCIDRs []string `json:"inboundCIDRs,omitempty"`

	// HealthCheck contains health check configuration
	// +optional
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`

	// PoolAlgorithm is the load balancing algorithm
	// +optional
	// +kubebuilder:validation:Enum=ROUND_ROBIN;LEAST_CONNECTIONS;SOURCE_IP
	PoolAlgorithm *string `json:"poolAlgorithm,omitempty"`

	// EnableAutoscale enables autoscaling for the load balancer
	// +optional
	EnableAutoscale *bool `json:"enableAutoscale,omitempty"`

	// Tags are key-value pairs for load balancer tagging
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// SecurityGroups are the security groups for the load balancer
	// +optional
	SecurityGroups []string `json:"securityGroups,omitempty"`

	// TargetNodeLabels are labels for targeting specific nodes
	// +optional
	TargetNodeLabels map[string]string `json:"targetNodeLabels,omitempty"`

	// IsPOC indicates if this is a proof of concept deployment
	// +optional
	IsPOC *bool `json:"isPOC,omitempty"`

	// PreferZoneID is the preferred zone ID for placement
	// +optional
	PreferZoneID *string `json:"preferZoneID,omitempty"`

	// PreferSubnetID is the preferred subnet ID for placement
	// +optional
	PreferSubnetID *string `json:"preferSubnetID,omitempty"`

	// Listeners defines the array of listeners for the load balancer
	// +optional
	Listeners []Listener `json:"listeners,omitempty"`

	// Pools defines the array of pools for the load balancer
	// +optional
	Pools []Pool `json:"pools,omitempty"`

	// Network contains configuration specific to network (L4) load balancers
	// This field is only valid when Type is "network"
	// +optional
	Network *NetworkLoadBalancerConfig `json:"network,omitempty"`

	// Application contains configuration specific to application (L7) load balancers
	// This field is only valid when Type is "application"
	// +optional
	Application *ApplicationLoadBalancerConfig `json:"application,omitempty"`
}

// VngcloudLoadBalancerConfigStatus defines the observed state of VngcloudLoadBalancerConfig.
type VngcloudLoadBalancerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// Message provides human-readable details about the current state
	// +optional
	Message *string `json:"message,omitempty"`

	// Reason provides a brief reason for the current state
	// +optional
	Reason *string `json:"reason,omitempty"`

	// Address is the DNS name or IP address assigned to the load balancer
	// +optional
	Address *string `json:"address,omitempty"`

	// UpdatedAt is the timestamp when the load balancer was last updated
	// +optional
	UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`

	// LoadBalancerID is the actual ID of the load balancer in VNG Cloud
	// +optional
	LoadBalancerID *string `json:"loadBalancerID,omitempty"`

	// LoadBalancerName is the actual name of the load balancer in VNG Cloud
	// +optional
	LoadBalancerName *string `json:"loadBalancerName,omitempty"`

	// ManagePools indicates if the controller should manage pools
	// +optional
	ManagePools *bool `json:"managePools,omitempty"`

	// ManageListeners indicates if the controller should manage listeners
	// +optional
	ManageListeners *bool `json:"manageListeners,omitempty"`

	// ManageDFPMembers indicates if the controller should manage DFP members
	// +optional
	ManageDFPMembers *bool `json:"manageDFPMembers,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// VngcloudLoadBalancerConfig is the Schema for the vngcloudloadbalancerconfigs API
type VngcloudLoadBalancerConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of VngcloudLoadBalancerConfig
	// +required
	Spec VngcloudLoadBalancerConfigSpec `json:"spec"`

	// status defines the observed state of VngcloudLoadBalancerConfig
	// +optional
	Status VngcloudLoadBalancerConfigStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// VngcloudLoadBalancerConfigList contains a list of VngcloudLoadBalancerConfig
type VngcloudLoadBalancerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VngcloudLoadBalancerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VngcloudLoadBalancerConfig{}, &VngcloudLoadBalancerConfigList{})
}
