package lbc

import (
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
)

// LoadBalancerConfigUtils to check if the object is supported by the controller
type LoadBalancerConfigUtils interface {
	// IsSupported returns true if the object is supported by the controller
	IsSupported(object *v1alpha1.LoadBalancerConfig) bool

	// IsPendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.LoadBalancerConfig) bool
}

func NewLoadBalancerConfigUtils(objectFinalizer string) LoadBalancerConfigUtils {
	return &defaultLBCUtils{
		objectFinalizer: objectFinalizer,
	}
}

type defaultLBCUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultLBCUtils) IsPendingFinalization(object *v1alpha1.LoadBalancerConfig) bool {
	return k8s.HasFinalizer(object, u.objectFinalizer)
}

// IsSupported returns true if the object is supported by the controller
func (u *defaultLBCUtils) IsSupported(object *v1alpha1.LoadBalancerConfig) bool {
	return object.DeletionTimestamp.IsZero()
}
