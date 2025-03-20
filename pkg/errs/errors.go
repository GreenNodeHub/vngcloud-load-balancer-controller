package errs

import (
	"fmt"
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

// The load balancer id lb-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx is not ready
func IsLoadBalancerNotReady(err error) bool {
	return strings.HasPrefix(err.Error(), "The load balancer id") && strings.HasSuffix(err.Error(), "is not ready")
}

var (
	ErrorNoImplementationSpecificConfigFound = fmt.Errorf("no implementation specific config found")
)

// if the error is due to load balancer not found
func IsGlobalLoadBalancerNotFound(err error) bool {
	return strings.EqualFold(err.Error(), "global_load_balancer_not_found")
}

// `create rule fail. SecurityGroupRuleExists`
func IsSecurityGroupRuleExists(err error) bool {
	return strings.Contains(err.Error(), "SecurityGroupRuleExists")
}

func IgnoreErrors(err error, funcs ...func(error) bool) error {
	if err == nil {
		return nil
	}
	for _, f := range funcs {
		if f(err) {
			return nil
		}
	}
	return err
}
