package lbc

import (
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// LBCUtils to check if the service is supported by the controller
type LBCUtils interface {
	// IsSupported returns true if the service is supported by the controller
	IsSupported(object *v1alpha1.LoadBalancerConfig) bool

	// IsPendingFinalization returns true if the service contains the aws-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.LoadBalancerConfig) bool
}

func NewLBCUtils(objectFinalizer string) *defaultLBCUtils {
	return &defaultLBCUtils{
		objectFinalizer: objectFinalizer,
	}
}

var _ LBCUtils = (*defaultLBCUtils)(nil)

type defaultLBCUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if service has the aws-load-balancer-controller finalizer
func (u *defaultLBCUtils) IsPendingFinalization(service *v1alpha1.LoadBalancerConfig) bool {
	return k8s.HasFinalizer(service, u.objectFinalizer)
}

// IsSupported returns true if the service is supported by the controller
func (u *defaultLBCUtils) IsSupported(service *v1alpha1.LoadBalancerConfig) bool {
	if !service.DeletionTimestamp.IsZero() {
		return false
	}
	return true
}
