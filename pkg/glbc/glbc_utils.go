package glbc

import (
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// GlobalLoadBalancerConfigUtils to check if the object is supported by the controller
type GlobalLoadBalancerConfigUtils interface {
	// IsSupported returns true if the object is supported by the controller
	IsSupported(object *v1alpha1.GlobalLoadBalancerConfig) bool

	// IsPendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.GlobalLoadBalancerConfig) bool
}

func NewGlobalLoadBalancerConfigUtils(objectFinalizer string) GlobalLoadBalancerConfigUtils {
	return &defaultGLBCUtils{
		objectFinalizer: objectFinalizer,
	}
}

type defaultGLBCUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultGLBCUtils) IsPendingFinalization(object *v1alpha1.GlobalLoadBalancerConfig) bool {
	return k8s.HasFinalizer(object, u.objectFinalizer)
}

// IsSupported returns true if the object is supported by the controller
func (u *defaultGLBCUtils) IsSupported(object *v1alpha1.GlobalLoadBalancerConfig) bool {
	return object.DeletionTimestamp.IsZero()
}
