package domain

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrorInvalidInput                        = errors.New("invalid input")
	ErrorNotImplemented                      = errors.New("not implemented yet")
	ErrorNotFound                            = errors.New("heheh not found")
	ErrorLoadBalancerStatusError             = errors.New("load balancer status is error")
	ErrorNoImplementationSpecificConfigFound = fmt.Errorf("no implementation specific config found")
	ErrorStatusNotUpdated                    = fmt.Errorf("status not updated")                               // when the status of CRD is not updated as expected
	ErrorSuperClientNotInitialized           = fmt.Errorf("super client not initialized, review your config") // when the super client is not initialized
)

const (
	RequestIcon = "🌐"
	WaitIcon    = "⏳"
	ReadyIcon   = "👍"
	SuccessIcon = "✅"
	ErrorIcon   = "❌"
	DebugIcon   = "🔰"
)

// if the error is due to load balancer not found
func IsLoadBalancerNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Production VNGCloud returns "Cannot get load balancer with id <id>";
	// the in-process mock and other internal repositories return the
	// generic domain.ErrorNotFound sentinel. Either form means the same
	// thing at this call site (LBC delete: GetLoadBalancerByID failed
	// because the LB is gone, so the delete is a no-op).
	return strings.HasPrefix(err.Error(), "Cannot get load balancer with id") ||
		errors.Is(err, ErrorNotFound)
}

// if the error is due to exceeded security group per server quota
func IsExceededSecurityGroupPerServerQuota(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Exceeded SEC_GROUP_PER_SERVER quota.")
}

// The load balancer id lb-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx is not ready
func IsLoadBalancerNotReady(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "The load balancer id") && strings.HasSuffix(err.Error(), "is not ready")
}

// if the error is due to load balancer not found
func IsGlobalLoadBalancerNotFound(err error) bool {
	return err != nil && strings.EqualFold(err.Error(), "global_load_balancer_not_found")
}

// `create rule fail. SecurityGroupRuleExists`
func IsSecurityGroupRuleExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SecurityGroupRuleExists")
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

// Cannot get security group with id secg-
func IsSecurityGroupNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "Cannot get security group with id secg-")
}

func IsRateLimitExceeded(err error) bool {
	// TODO
	return false
}

// Cannot get server with id ins-...
func IsServerNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "Cannot get server with id")
}
