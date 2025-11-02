package nsg

import (
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// NodeSecurityGroupUtils to check if the object is supported by the controller
type NodeSecurityGroupUtils interface {
	// IsSupported returns true if the object is supported by the controller
	IsSupported(object *v1alpha1.NodeSecurityGroup) bool

	// IsPendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.NodeSecurityGroup) bool
}

func NewNodeSecurityGroupUtils(objectFinalizer string) NodeSecurityGroupUtils {
	return &defaultNSGUtils{
		objectFinalizer: objectFinalizer,
	}
}

type defaultNSGUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultNSGUtils) IsPendingFinalization(object *v1alpha1.NodeSecurityGroup) bool {
	return k8s.HasFinalizer(object, u.objectFinalizer)
}

// IsSupported returns true if the object is supported by the controller
func (u *defaultNSGUtils) IsSupported(object *v1alpha1.NodeSecurityGroup) bool {
	return object.DeletionTimestamp.IsZero()
}
