package ingress_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

// An Ingress may reference a Service that does not exist - a typo, or a chart whose
// Service has not been applied yet. That must cost the caller only that one path, so
// buildPool reports it as errBackendUnresolvable and every other error stays fatal: a
// transient API failure must not silently drop routes off the load balancer.
func TestBuildPoolReportsMissingServiceAsSkippable(t *testing.T) {
	tests := []struct {
		name     string
		getErr   error
		wantSkip bool
		wantErr  bool
	}{
		{
			name:     "service does not exist - skippable",
			getErr:   apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, "nfk"),
			wantSkip: true,
			wantErr:  true,
		},
		{
			name:     "any other failure stays fatal",
			getErr:   apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, "nfk", errors.New("nope")),
			wantSkip: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockK8s := repository.NewMockK8sRepository(t)
			mockK8s.EXPECT().
				GetService(mock.Anything, mock.Anything).
				Return(nil, tt.getErr)

			task := &defaultModelBuildTask{
				logger: logrus.NewEntry(logrus.New()),
				ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "prod-magnet", Namespace: "magnet-common-sit"},
				},
				k8sRepo: mockK8s,
			}

			pool, err := task.buildPool(context.Background(),
				&networkingv1.IngressServiceBackend{
					Name: "nfk",
					Port: networkingv1.ServiceBackendPort{Number: 443},
				}, nil)

			assert.Nil(t, pool)
			if tt.wantErr {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.wantSkip, errors.Is(err, errBackendUnresolvable),
				"errors.Is(err, errBackendUnresolvable) decides whether the caller skips the path or fails the whole Ingress")
		})
	}
}

// The other way an Ingress can name a backend that cannot be resolved: the Service is there,
// but it does not serve the port the rule asks for. Nothing will change until the Ingress or
// the Service does, and it is one path's problem - so it is skippable for the same reason a
// missing Service is. Reported as a whole-Ingress failure, it would leave an Ingress with one
// typo'd port with no routing at all.
func TestBuildPoolReportsAPortTheServiceDoesNotServeAsSkippable(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "nfk", Namespace: "magnet-common-sit"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80, NodePort: 30080}},
		},
	}

	tests := []struct {
		name string
		port networkingv1.ServiceBackendPort
	}{
		{
			name: "port number the Service does not serve",
			port: networkingv1.ServiceBackendPort{Number: 443},
		},
		{
			name: "port name the Service does not serve",
			port: networkingv1.ServiceBackendPort{Name: "https"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockK8s := repository.NewMockK8sRepository(t)
			mockK8s.EXPECT().
				GetService(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _ types.NamespacedName) (*corev1.Service, error) {
					return svc.DeepCopy(), nil
				})

			task := &defaultModelBuildTask{
				logger: logrus.NewEntry(logrus.New()),
				ingress: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: "prod-magnet", Namespace: "magnet-common-sit"},
				},
				k8sRepo: mockK8s,
			}

			pool, err := task.buildPool(context.Background(),
				&networkingv1.IngressServiceBackend{Name: "nfk", Port: tt.port}, nil)

			assert.Nil(t, pool)
			assert.Error(t, err)
			assert.ErrorIs(t, err, errBackendUnresolvable,
				"a port the Service does not serve must be skippable, not fatal")
		})
	}
}

// backendPortDescription is what the log and the error say was asked for, so it has to render
// whichever half of ServiceBackendPort the Ingress filled in.
func TestBackendPortDescription(t *testing.T) {
	assert.Equal(t, `"https"`, backendPortDescription(networkingv1.ServiceBackendPort{Name: "https"}))
	assert.Equal(t, "8443", backendPortDescription(networkingv1.ServiceBackendPort{Number: 8443}))
}

func newBuildTaskForListenerAcl(anns map[string]string) *defaultModelBuildTask {
	return &defaultModelBuildTask{
		logger: logrus.NewEntry(logrus.New()),
		ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", Annotations: anns},
		},
		annotationParser: annotations.NewSuffixAnnotationParser(domain.INGRESS_ANNOTATION_PREFIX),
	}
}

// Both listeners of an Ingress carry the same ACL, so the HTTP and HTTPS front doors agree.
func TestBuildListenersCarryTheAcl(t *testing.T) {
	task := newBuildTaskForListenerAcl(map[string]string{
		domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixDroppedCIDRs:     "203.0.113.9/32",
		domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixACLDefaultAction: "drop",
	})

	for _, isHttps := range []bool{false, true} {
		l, err := task.buildListeners(context.Background(), isHttps)
		require.NoError(t, err)
		assert.Nil(t, l.AllowedCidrs)
		assert.Equal(t, ptr.To("203.0.113.9/32"), l.BlockedCidrs)
		assert.Equal(t, ptr.To("drop"), l.DefaultAction)
	}
}

// An allow list alone means "nobody else", so the listener must carry the drop default.
func TestBuildListenersDefaultToDropWhenOnlyAnAllowListIsSet(t *testing.T) {
	task := newBuildTaskForListenerAcl(map[string]string{
		domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixInboundCIDRs: "198.51.100.0/24",
	})

	l, err := task.buildListeners(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, ptr.To("198.51.100.0/24"), l.AllowedCidrs)
	assert.Nil(t, l.BlockedCidrs)
	assert.Equal(t, ptr.To("drop"), l.DefaultAction)
}

func TestBuildListenersWithoutAclAnnotationsLeaveTheAclUnmanaged(t *testing.T) {
	task := newBuildTaskForListenerAcl(nil)

	l, err := task.buildListeners(context.Background(), false)
	require.NoError(t, err)
	assert.Nil(t, l.AllowedCidrs)
	assert.Nil(t, l.BlockedCidrs)
	assert.Nil(t, l.DefaultAction)
}

func TestBuildListenersRejectsABadDroppedList(t *testing.T) {
	task := newBuildTaskForListenerAcl(map[string]string{
		domain.INGRESS_ANNOTATION_PREFIX + "/" + annotations.SuffixDroppedCIDRs: "nope",
	})

	_, err := task.buildListeners(context.Background(), false)
	require.Error(t, err, "a bad block list must stop the build so the last good spec stays")
}
