package errs

import "errors"

var (
	ErrorNoConfig = errors.New("config is nil")

	ErrorServicePortEmpty      = errors.New("service port is empty")
	ErrorNodeNotHaveInternalIP = errors.New("node not have internal IP")
	ErrorNoNetworkInfo         = errors.New("no network info, lack of networkID or subnetID or subnetCIDR")
	ErrorNoNodeAtInitTime      = errors.New("require at least 1 node to get network information")
	ErrorServicePortNameEmpty  = errors.New("service port name is empty")
	ErrorServicePortNotFound   = errors.New("service port not found")

	ErrorLoadBalancerNotHaveUUID        = errors.New("load balancer not have UUID after find by name or create, need to retry")
	ErrorLoadBalancerStatusError        = errors.New("load balancer status is error")
	ErrorLoadBalancerNotHaveInformation = errors.New("load balancer not have information")

	ErrorInvalidInput   = errors.New("invalid input")
	ErrorNotImplemented = errors.New("not implemented yet")
	ErrorNotFound       = errors.New("not found")

	ErrorMissingCertificates = errors.New("missing certificates, need to specific through annotaion")
)
