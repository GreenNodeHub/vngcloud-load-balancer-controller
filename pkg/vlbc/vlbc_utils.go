package vlbc

import (
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// VLBCUtils to check if the service is supported by the controller
type VLBCUtils interface {
	// IsSupported returns true if the service is supported by the controller
	IsSupported(service *v1alpha1.VngcloudLoadBalancerConfig) bool

	// IsPendingFinalization returns true if the service contains the aws-load-balancer-controller finalizer
	IsPendingFinalization(service *v1alpha1.VngcloudLoadBalancerConfig) bool
}

func NewVLBCUtils(serviceFinalizer string) *defaultVLBCUtils {
	return &defaultVLBCUtils{
		serviceFinalizer: serviceFinalizer,
	}
}

var _ VLBCUtils = (*defaultVLBCUtils)(nil)

type defaultVLBCUtils struct {
	serviceFinalizer string
}

// IsPendingFinalization returns true if service has the aws-load-balancer-controller finalizer
func (u *defaultVLBCUtils) IsPendingFinalization(service *v1alpha1.VngcloudLoadBalancerConfig) bool {
	return k8s.HasFinalizer(service, u.serviceFinalizer)
}

// IsSupported returns true if the service is supported by the controller
func (u *defaultVLBCUtils) IsSupported(service *v1alpha1.VngcloudLoadBalancerConfig) bool {
	if !service.DeletionTimestamp.IsZero() {
		return false
	}
	return true
}
