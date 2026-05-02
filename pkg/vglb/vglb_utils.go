package vglb

import (
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
)

// VngcloudGlobalLoadBalancerUtils to check if the object is supported by the controller
type VngcloudGlobalLoadBalancerUtils interface {
	// IsSupported returns true if the object is supported by the controller
	IsSupported(object *v1alpha1.VngcloudGlobalLoadBalancer) bool

	// IsPendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.VngcloudGlobalLoadBalancer) bool
}

func NewVngcloudGlobalLoadBalancerUtils(objectFinalizer string) VngcloudGlobalLoadBalancerUtils {
	return &defaultVGLBUtils{
		objectFinalizer: objectFinalizer,
	}
}

type defaultVGLBUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultVGLBUtils) IsPendingFinalization(object *v1alpha1.VngcloudGlobalLoadBalancer) bool {
	return k8s.HasFinalizer(object, u.objectFinalizer)
}

// IsSupported returns true if the object is supported by the controller
func (u *defaultVGLBUtils) IsSupported(object *v1alpha1.VngcloudGlobalLoadBalancer) bool {
	return object.DeletionTimestamp.IsZero()
}
