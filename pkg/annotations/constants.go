package annotations

const (
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

	// for l4 only
	SuffixEnableProxyProtocol = "enable-proxy-protocol"

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

	SuffixIsPOC  = "isPOC"  // is poc
	SuffixIsPOC2 = "is-poc" // is poc but deprecated

	SuffixTrigger = "trigger" // trigger

	SuffixPreferZoneID   = "prefer-zone-id"   // prefer zone id
	SuffixPreferSubnetID = "prefer-subnet-id" // prefer subnet id
)
