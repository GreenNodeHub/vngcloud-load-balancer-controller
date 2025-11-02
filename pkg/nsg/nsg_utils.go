package nsg

import (
	"github.com/anngdinh/operator-helper/k8s"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// NSGUtils to check if the service is supported by the controller
type NSGUtils interface {
	// IsSupported returns true if the service is supported by the controller
	IsSupported(object *v1alpha1.NodeSecurityGroup) bool

	// IsPendingFinalization returns true if the service contains the aws-load-balancer-controller finalizer
	IsPendingFinalization(object *v1alpha1.NodeSecurityGroup) bool
}

func NewNSGUtils(objectFinalizer string) *defaultNSGUtils {
	return &defaultNSGUtils{
		objectFinalizer: objectFinalizer,
	}
}

var _ NSGUtils = (*defaultNSGUtils)(nil)

type defaultNSGUtils struct {
	objectFinalizer string
}

// IsPendingFinalization returns true if service has the aws-load-balancer-controller finalizer
func (u *defaultNSGUtils) IsPendingFinalization(service *v1alpha1.NodeSecurityGroup) bool {
	return k8s.HasFinalizer(service, u.objectFinalizer)
}

// IsSupported returns true if the service is supported by the controller
func (u *defaultNSGUtils) IsSupported(service *v1alpha1.NodeSecurityGroup) bool {
	if !service.DeletionTimestamp.IsZero() {
		return false
	}
	return true
}
