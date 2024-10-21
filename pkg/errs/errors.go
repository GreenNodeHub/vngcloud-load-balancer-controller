package errs

import (
	"errors"
	"strings"
)

var (
	ErrorNoConfig = errors.New("config is nil")

	ErrorServicePortEmpty     = errors.New("service port is empty")
	ErrorNoNetworkInfo        = errors.New("no network info, lack of networkID or subnetID or subnetCIDR")
	ErrorNoNodeAtInitTime     = errors.New("require at least 1 node to get network information")
	ErrorServicePortNameEmpty = errors.New("service port name is empty")
	ErrorServicePortNotFound  = errors.New("service port not found")

	ErrorLoadBalancerNotHaveUUID        = errors.New("load balancer not have UUID after find by name or create, need to retry")
	ErrorLoadBalancerStatusError        = errors.New("load balancer status is error")
	ErrorLoadBalancerNotHaveInformation = errors.New("load balancer not have information")

	ErrorInvalidInput   = errors.New("invalid input")
	ErrorNotImplemented = errors.New("not implemented yet")
	ErrorNotFound       = errors.New("not found")

	ErrorMissingCertificates = errors.New("missing certificates, need to specific through annotation")

	ErrorSecurityGroupNotFound = errors.New("security group not found")
	ErrorSecurityGroupInUse    = errors.New("security group in use")
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
