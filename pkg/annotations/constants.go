package annotations

const (
	// for all
	SuffixLoadBalancerName           = "load-balancer-name"           // only set via the annotation
	SuffixPackageID                  = "package-id"                   // size of lb
	SuffixScheme                     = "scheme"                       // internal or external
	SuffixTargetType                 = "target-type"                  // instance or ip
	SuffixLoadBalancerID             = "load-balancer-id"             // managed by the controller
	SuffixIgnore                     = "ignore"                       // ignore the resource
	SuffixIdleTimeoutClient          = "idle-timeout-client"          // idle timeout for client
	SuffixIdleTimeoutMember          = "idle-timeout-member"          // idle timeout for member
	SuffixIdleTimeoutConnection      = "idle-timeout-connection"      // idle timeout for connection
	SuffixInboundCIDRs               = "inbound-cidrs"                // inbound CIDRs
	SuffixHealthcheckPort            = "healthcheck-port"             // healthcheck port
	SuffixHealthcheckProtocol        = "healthcheck-protocol"         // healthcheck protocol
	SuffixSuccessCodes               = "success-codes"                // success codes,                	only for http/https healthcheck protocol
	SuffixHealthcheckPath            = "healthcheck-path"             // healthcheck path, 				only for http/https healthcheck protocol
	SuffixHealthcheckHttpMethod      = "healthcheck-http-method"      // healthcheck http method, 		only for http/https healthcheck protocol
	SuffixHealthcheckHttpVersion     = "healthcheck-http-version"     // healthcheck http version, 		only for http/https healthcheck protocol
	SuffixHealthcheckHttpDomainName  = "healthcheck-http-domain-name" // healthcheck http domain name, 	only for http/https healthcheck protocol
	SuffixHealthcheckIntervalSeconds = "healthcheck-interval-seconds" // healthcheck interval seconds
	SuffixHealthcheckTimeoutSeconds  = "healthcheck-timeout-seconds"  // healthcheck timeout seconds
	SuffixHealthyThresholdCount      = "healthy-threshold-count"      // healthy threshold count
	SuffixUnhealthyThresholdCount    = "unhealthy-threshold-count"    // unhealthy threshold count
	SuffixPoolAlgorithm              = "pool-algorithm"               // pool algorithm
	SuffixEnableAutoscale            = "enable-autoscale"             // enable autoscale
	SuffixTags                       = "tags"                         // tags
	SuffixSecurityGroups             = "security-groups"              // security groups
	SuffixTargetNodeLabels           = "target-node-labels"           // target node labels
	SuffixIsPOC                      = "isPOC"                        // is poc
	SuffixIsPOC2                     = "is-poc"                       // is poc but deprecated
	SuffixPreferZoneID               = "prefer-zone-id"               // prefer zone id
	SuffixPreferSubnetID             = "prefer-subnet-id"             // prefer subnet id

	// for non-LoadBalancer service type support
	// Enable load balancer for NodePort/ClusterIP service types
	// For ClusterIP: only works with Cilium native routing, target type always IP
	SuffixEnableLoadBalancer = "enable-load-balancer"

	// for l4 only
	SuffixEnableProxyProtocol = "enable-proxy-protocol"

	// for l4 inter-vpc only
	SuffixPrivateSubnetID = "private-subnet-id" // new annotation
	SuffixBackendSubnetID = "backend-subnet-id" // deprecated, use private-subnet-id instead
	SuffixPrivateZoneID   = "private-zone-id"   // zone of the client subnet for InterVPC

	// for l7 only
	SuffixEnableStickySession          = "enable-sticky-session"
	SuffixEnableTLSEncryption          = "enable-tls-encryption"
	SuffixCertificateIDs               = "certificate-ids"
	SuffixImplementationSpecificParams = "implementation-specific-params"
	SuffixHeader                       = "header"
	SuffixInsertHeaders                = "insert-headers"
	SuffixClientCertificateID          = "client-certificate-id"
	SuffixAutoReorderPolicies          = "auto-reorder-policies"

	// for management
	SuffixManagePools      = "manage-pools"
	SuffixManageListeners  = "manage-listeners"
	SuffixManageDFPMembers = "manage-dfp-members"

	SuffixTrigger = "trigger" // trigger

	// for global load balancer
	SuffixDescription = "description" // description for the resource

	// for Service GLB (glb.vks.vngcloud.vn prefix)
	SuffixGLBEnable = "enable" // glb.vks.vngcloud.vn/enable — enable GLB for this Service

	// for Service GLB annotation suffixes (glb.vks.vngcloud.vn prefix)
	// These mirror shared suffixes but are dedicated to the GLB controller for clarity.
	SuffixGLBLoadBalancerID             = "load-balancer-id"
	SuffixGLBLoadBalancerName           = "load-balancer-name"
	SuffixGLBPackageID                  = "package-id"
	SuffixGLBDescription                = "description"
	SuffixGLBTargetType                 = "target-type"
	SuffixGLBHealthcheckPort            = "healthcheck-port"
	SuffixGLBPoolAlgorithm              = "pool-algorithm"
	SuffixGLBIdleTimeoutClient          = "idle-timeout-client"
	SuffixGLBIdleTimeoutMember          = "idle-timeout-member"
	SuffixGLBIdleTimeoutConnection      = "idle-timeout-connection"
	SuffixGLBInboundCIDRs               = "inbound-cidrs"
	SuffixGLBEnableProxyProtocol        = "enable-proxy-protocol"
	SuffixGLBHealthcheckProtocol        = "healthcheck-protocol"
	SuffixGLBHealthyThresholdCount      = "healthy-threshold-count"
	SuffixGLBUnhealthyThresholdCount    = "unhealthy-threshold-count"
	SuffixGLBHealthcheckIntervalSeconds = "healthcheck-interval-seconds"
	SuffixGLBHealthcheckTimeoutSeconds  = "healthcheck-timeout-seconds"
	SuffixGLBHealthcheckHttpMethod      = "healthcheck-http-method"
	SuffixGLBHealthcheckPath            = "healthcheck-path"
	SuffixGLBSuccessCodes               = "success-codes"
	SuffixGLBHealthcheckHttpVersion     = "healthcheck-http-version"
	SuffixGLBHealthcheckHttpDomainName  = "healthcheck-http-domain-name"
)
