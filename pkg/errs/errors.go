package errs

import "errors"

var (
	ErrorNoConfig = errors.New("config is nil")

	ErrorServicePortEmpty      = errors.New("service port is empty")
	ErrorNodeNotHaveInternalIP = errors.New("node not have internal IP")
	ErrorNoNetworkInfo         = errors.New("no network info, lack of networkID or subnetID or subnetCIDR")
	ErrorNoNodeAtInitTime      = errors.New("require at least 1 node to get network information")

	ErrorLoadBalancerNotHaveUUID        = errors.New("load balancer not have UUID after find by name or create, need to retry")
	ErrorLoadBalancerStatusError        = errors.New("load balancer status is error")
	ErrorLoadBalancerNotHaveInformation = errors.New("load balancer not have information")

	ErrorInvalidInput   = errors.New("invalid input")
	ErrorNotImplemented = errors.New("not implemented yet")
	ErrorNotFound       = errors.New("not found")
)
