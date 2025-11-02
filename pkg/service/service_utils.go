package service

import (
	"github.com/anngdinh/operator-helper/k8s"
	corev1 "k8s.io/api/core/v1"
)

// ServiceUtils to check if the object is supported by the controller
type ServiceUtils interface {
	// IsServiceSupported returns true if the object is supported by the controller
	IsServiceSupported(object *corev1.Service) bool

	// IsServicePendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsServicePendingFinalization(object *corev1.Service) bool
}

func NewServiceUtils(serviceFinalizer string) ServiceUtils {
	return &defaultServiceUtils{
		serviceFinalizer: serviceFinalizer,
	}
}

type defaultServiceUtils struct {
	serviceFinalizer string
}

// IsServicePendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultServiceUtils) IsServicePendingFinalization(object *corev1.Service) bool {
	return k8s.HasFinalizer(object, u.serviceFinalizer)
}

// IsServiceSupported returns true if the object is supported by the controller
func (u *defaultServiceUtils) IsServiceSupported(object *corev1.Service) bool {
	if !object.DeletionTimestamp.IsZero() {
		return false
	}
	if object.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	return true
}
