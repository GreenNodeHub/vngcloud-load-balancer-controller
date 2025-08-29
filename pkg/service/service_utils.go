package service

import (
	"github.com/anngdinh/operator-helper/k8s"
	corev1 "k8s.io/api/core/v1"
)

// ServiceUtils to check if the service is supported by the controller
type ServiceUtils interface {
	// IsServiceSupported returns true if the service is supported by the controller
	IsServiceSupported(service *corev1.Service) bool

	// IsServicePendingFinalization returns true if the service contains the aws-load-balancer-controller finalizer
	IsServicePendingFinalization(service *corev1.Service) bool
}

func NewServiceUtils(serviceFinalizer string) *defaultServiceUtils {
	return &defaultServiceUtils{
		serviceFinalizer: serviceFinalizer,
	}
}

var _ ServiceUtils = (*defaultServiceUtils)(nil)

type defaultServiceUtils struct {
	serviceFinalizer string
}

// IsServicePendingFinalization returns true if service has the aws-load-balancer-controller finalizer
func (u *defaultServiceUtils) IsServicePendingFinalization(service *corev1.Service) bool {
	return k8s.HasFinalizer(service, u.serviceFinalizer)
}

// IsServiceSupported returns true if the service is supported by the controller
func (u *defaultServiceUtils) IsServiceSupported(service *corev1.Service) bool {
	if !service.DeletionTimestamp.IsZero() {
		return false
	}
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	return true
}
