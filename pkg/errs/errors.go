package errs

import (
	"strings"
)

// if the error is due to load balancer not found
func IsLoadBalancerNotFound(err error) bool {
	// if have prefix "Cannot get load balancer with id" then consider as not found
	return strings.HasPrefix(err.Error(), "Cannot get load balancer with id")
}

// if the error is due to exceeded security group per server quota
func IsExceededSecurityGroupPerServerQuota(err error) bool {
	return strings.Contains(err.Error(), "Exceeded SEC_GROUP_PER_SERVER quota.")
}
