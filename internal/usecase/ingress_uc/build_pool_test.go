package ingress_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// An Ingress may reference a Service that does not exist - a typo, or a chart whose
// Service has not been applied yet. That must cost the caller only that one path, so
// buildPool reports it as errBackendNotFound and every other error stays fatal: a
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
			assert.Equal(t, tt.wantSkip, errors.Is(err, errBackendNotFound),
				"errors.Is(err, errBackendNotFound) decides whether the caller skips the path or fails the whole Ingress")
		})
	}
}
