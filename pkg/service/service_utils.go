package service

import (
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
)

// ServiceUtils to check if the object is supported by the controller
type ServiceUtils interface {
	// IsServiceSupported returns true if the object is supported by the controller
	IsServiceSupported(object *corev1.Service) bool

	// IsServicePendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsServicePendingFinalization(object *corev1.Service) bool
}

func NewServiceUtils(serviceFinalizer string, annotationParser annotations.Parser) ServiceUtils {
	return &defaultServiceUtils{
		serviceFinalizer: serviceFinalizer,
		annotationParser: annotationParser,
	}
}

type defaultServiceUtils struct {
	serviceFinalizer string
	annotationParser annotations.Parser
}

// IsServicePendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultServiceUtils) IsServicePendingFinalization(object *corev1.Service) bool {
	return k8s.HasFinalizer(object, u.serviceFinalizer)
}

// IsServiceSupported returns true if the object is supported by the controller
// Supports:
// - LoadBalancer type services
// - NodePort type services with enable-lb annotation
// - ClusterIP type services with enable-lb annotation (only works with Cilium native routing, target type always IP)
func (u *defaultServiceUtils) IsServiceSupported(object *corev1.Service) bool {
	if !object.DeletionTimestamp.IsZero() {
		return false
	}
	// Always support LoadBalancer type
	if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return true
	}
	// Support NodePort/ClusterIP type with enable-lb annotation
	if object.Spec.Type == corev1.ServiceTypeNodePort || object.Spec.Type == corev1.ServiceTypeClusterIP {
		return u.isLBEnabled(object)
	}
	return false
}

// isLBEnabled checks if the service has enable-lb annotation set to true
func (u *defaultServiceUtils) isLBEnabled(object *corev1.Service) bool {
	if u.annotationParser == nil {
		return false
	}
	enabled := false
	u.annotationParser.ParseBoolAnnotation(annotations.SuffixEnableLoadBalancer, &enabled, object.Annotations)
	return enabled
}
