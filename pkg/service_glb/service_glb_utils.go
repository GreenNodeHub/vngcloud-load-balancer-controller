package service_glb

import (
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
)

// ServiceGLBUtils checks if a Service should be managed by the GLB controller.
// Unlike ServiceUtils, it does NOT filter on ServiceType — any service type can be
// GLB-enabled via the glb.vks.vngcloud.vn/enable annotation.
type ServiceGLBUtils interface {
	// IsServiceGLBSupported returns true if the Service has glb.vks.vngcloud.vn/enable=true
	// and is not being deleted.
	IsServiceGLBSupported(object *corev1.Service) bool

	// IsServiceGLBPendingFinalization returns true if the Service has the GLB finalizer.
	IsServiceGLBPendingFinalization(object *corev1.Service) bool
}

// NewServiceGLBUtils creates a new ServiceGLBUtils.
func NewServiceGLBUtils(serviceFinalizer string, annotationParser annotations.Parser) ServiceGLBUtils {
	return &defaultServiceGLBUtils{
		serviceFinalizer: serviceFinalizer,
		annotationParser: annotationParser,
	}
}

type defaultServiceGLBUtils struct {
	serviceFinalizer string
	annotationParser annotations.Parser
}

// IsServiceGLBPendingFinalization returns true if the Service has the GLB finalizer.
func (u *defaultServiceGLBUtils) IsServiceGLBPendingFinalization(object *corev1.Service) bool {
	return k8s.HasFinalizer(object, u.serviceFinalizer)
}

// IsServiceGLBSupported returns true if the Service is enabled for GLB management.
// A Service is supported if:
//   - It is not being deleted (DeletionTimestamp is zero)
//   - It has the annotation glb.vks.vngcloud.vn/enable=true
func (u *defaultServiceGLBUtils) IsServiceGLBSupported(object *corev1.Service) bool {
	if !object.DeletionTimestamp.IsZero() {
		return false
	}
	enabled := false
	u.annotationParser.ParseBoolAnnotation(annotations.SuffixGLBEnable, &enabled, object.Annotations)
	return enabled
}
