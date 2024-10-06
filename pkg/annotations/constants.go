package annotations

const (

	// Ingress annotation suffixes
	IngressAnnotationIgnore = "ignore"

	// IngressAnnotationSubnetID              = "subnet-id"  // both annotation and cloud-config
	// IngressAnnotationNetworkID             = "network-id" // both annotation and cloud-config
	// IngressAnnotationOwnedListeners        = "owned-listeners"
	// IngressAnnotationCloudLoadBalancerName = "cloud-loadbalancer-name" // set via annotation
	// IngressAnnotationLoadBalancerOwner     = "load-balancer-owner"

	// Node annotations
	IngressAnnotationTargetNodeLabels = "target-node-labels"

	// LB annotations
	IngressAnnotationLoadBalancerID   = "load-balancer-id"
	IngressAnnotationLoadBalancerName = "load-balancer-name" // only set via the annotation
	IngressAnnotationPackageID        = "package-id"         // both annotation and cloud-config
	IngressAnnotationSecurityGroups   = "security-groups"
	IngressAnnotationTags             = "tags"
	IngressAnnotationScheme           = "scheme"
	IngressAnnotationCertificateIDs   = "certificate-ids"
	IngressAnnotationEnableAutoscale  = "enable-autoscale"

	// Listener annotations
	IngressAnnotationIdleTimeoutClient     = "idle-timeout-client"     // both annotation and cloud-config
	IngressAnnotationIdleTimeoutMember     = "idle-timeout-member"     // both annotation and cloud-config
	IngressAnnotationIdleTimeoutConnection = "idle-timeout-connection" // both annotation and cloud-config
	IngressAnnotationInboundCIDRs          = "inbound-cidrs"

	// Pool annotations
	IngressAnnotationPoolAlgorithm       = "pool-algorithm" // both annotation and cloud-config
	IngressAnnotationEnableStickySession = "enable-sticky-session"
	IngressAnnotationEnableTLSEncryption = "enable-tls-encryption"
	IngressAnnotationHealthcheckPort     = "healthcheck-port"

	// Pool healthcheck annotations
	IngressAnnotationHealthcheckProtocol        = "healthcheck-protocol"
	IngressAnnotationHealthcheckIntervalSeconds = "healthcheck-interval-seconds"
	IngressAnnotationHealthcheckTimeoutSeconds  = "healthcheck-timeout-seconds"
	IngressAnnotationHealthyThresholdCount      = "healthy-threshold-count"
	IngressAnnotationUnhealthyThresholdCount    = "unhealthy-threshold-count"

	// Pool healthcheck annotations for HTTP
	IngressAnnotationHealthcheckPath           = "healthcheck-path"
	IngressAnnotationSuccessCodes              = "success-codes"
	IngressAnnotationHealthcheckHttpMethod     = "healthcheck-http-method"
	IngressAnnotationHealthcheckHttpVersion    = "healthcheck-http-version"
	IngressAnnotationHealthcheckHttpDomainName = "healthcheck-http-domain-name"

	// NLB annotation suffixes
	ServiceAnnotationIgnore = "/ignore"

	// ServiceAnnotationSubnetID              = "/subnet-id"  // both annotation and cloud-config
	// ServiceAnnotationNetworkID             = "/network-id" // both annotation and cloud-config
	// ServiceAnnotationOwnedListeners        = "/owned-listeners"
	// ServiceAnnotationCloudLoadBalancerName = "/cloud-loadbalancer-name" // set via annotation
	// ServiceAnnotationLoadBalancerOwner     = "/load-balancer-owner"

	// // Node annotations
	ServiceAnnotationTargetNodeLabels = "/target-node-labels"

	// // LB annotations
	ServiceAnnotationLoadBalancerID   = "/load-balancer-id"
	ServiceAnnotationLoadBalancerName = "/load-balancer-name" // only set via the annotation
	ServiceAnnotationPackageID        = "/package-id"         // both annotation and cloud-config
	ServiceAnnotationSecurityGroups   = "/security-groups"
	ServiceAnnotationTags             = "/tags"
	ServiceAnnotationScheme           = "/scheme"
	ServiceAnnotationEnableAutoscale  = "/enable-autoscale"

	// // Listener annotations
	ServiceAnnotationIdleTimeoutClient     = "/idle-timeout-client"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutMember     = "/idle-timeout-member"     // both annotation and cloud-config
	ServiceAnnotationIdleTimeoutConnection = "/idle-timeout-connection" // both annotation and cloud-config
	ServiceAnnotationInboundCIDRs          = "/inbound-cidrs"

	// // Pool annotations
	ServiceAnnotationPoolAlgorithm   = "/pool-algorithm" // both annotation and cloud-config
	ServiceAnnotationProxyProtocol   = "/enable-proxy-protocol"
	ServiceAnnotationHealthcheckPort = "/healthcheck-port"
	// ServiceAnnotationEnableStickySession = "/enable-sticky-session"
	// ServiceAnnotationEnableTLSEncryption = "/enable-tls-encryption"

	// // Pool healthcheck annotations
	ServiceAnnotationHealthcheckProtocol        = "/healthcheck-protocol"
	ServiceAnnotationHealthcheckIntervalSeconds = "/healthcheck-interval-seconds"
	ServiceAnnotationHealthcheckTimeoutSeconds  = "/healthcheck-timeout-seconds"
	ServiceAnnotationHealthyThresholdCount      = "/healthy-threshold-count"
	ServiceAnnotationUnhealthyThresholdCount    = "/unhealthy-threshold-count"

	// // Pool healthcheck annotations for HTTP
	ServiceAnnotationHealthcheckPath           = "/healthcheck-path"
	ServiceAnnotationSuccessCodes              = "/success-codes"
	ServiceAnnotationHealthcheckHttpMethod     = "/healthcheck-http-method"
	ServiceAnnotationHealthcheckHttpVersion    = "/healthcheck-http-version"
	ServiceAnnotationHealthcheckHttpDomainName = "/healthcheck-http-domain-name"

	SuffixTargetType       = "target-type"        // instance or ip
	SuffixLoadBalancerID   = "load-balancer-id"   // managed by the controller
	SuffixLoadBalancerName = "load-balancer-name" // only set via the annotation
	SuffixIgnore           = "ignore"             // ignore the resource
)
