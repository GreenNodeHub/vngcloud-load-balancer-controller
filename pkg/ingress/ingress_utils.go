package ingress

import (
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	networkingv1 "k8s.io/api/networking/v1"
)

const (
	// IngressClass specifies which Ingress class we accept
	IngressClass = "vngcloud"
)

// IngressUtils to check if the object is supported by the controller
type IngressUtils interface {
	// IsIngressSupported returns true if the object is supported by the controller
	IsIngressSupported(object *networkingv1.Ingress) bool

	// IsIngressPendingFinalization returns true if the object contains the vngcloud-load-balancer-controller finalizer
	IsIngressPendingFinalization(object *networkingv1.Ingress) bool
}

func NewIngressUtils(objectFinalizer string) IngressUtils {
	return &defaultIngressUtils{
		objectFinalizer: objectFinalizer,
	}
}

type defaultIngressUtils struct {
	objectFinalizer string
}

// IsIngressPendingFinalization returns true if object has the vngcloud-load-balancer-controller finalizer
func (u *defaultIngressUtils) IsIngressPendingFinalization(object *networkingv1.Ingress) bool {
	return k8s.HasFinalizer(object, u.objectFinalizer)
}

// IsIngressSupported returns true if the object is supported by the controller
func (u *defaultIngressUtils) IsIngressSupported(object *networkingv1.Ingress) bool {
	if !object.DeletionTimestamp.IsZero() {
		return false
	}
	if object.Spec.IngressClassName != nil && *object.Spec.IngressClassName != IngressClass {
		return false
	}
	return true
}
