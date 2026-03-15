package glbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// TestDeployListener_PopulatesName verifies BUG-04:
// The returned CreatedGlobalListener must have its Name populated from the API entity.
// Previously the Name field was missing from the return struct, leaving it always empty.
func TestDeployListener_PopulatesName(t *testing.T) {
	tests := []struct {
		name             string
		listenerSpec     v1alpha1.GlobalListener
		existingListener *entityv2.GlobalListener
		setupMock        func(*repository.MockVngCloudRepository)
		wantName         string
		wantId           string
		wantPort         int
	}{
		{
			name: "existing listener name is populated in returned CreatedGlobalListener",
			listenerSpec: v1alpha1.GlobalListener{
				Name:         "test-listener",
				ProtocolPort: 80,
				Protocol:     "TCP",
			},
			existingListener: &entityv2.GlobalListener{
				ID:       "lis-123",
				Name:     "test-listener",
				Port:     80,
				Protocol: "TCP",
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				// buildListenerUpdateRequest: no update needed (protocol matches, no changes)
				// -> no UpdateGlobalListener call, no WaitGlobalLoadBalancerActive call
			},
			wantName: "test-listener",
			wantId:   "lis-123",
			wantPort: 80,
		},
		{
			name: "existing listener with different name is populated from API response",
			listenerSpec: v1alpha1.GlobalListener{
				Name:         "listener-port-443",
				ProtocolPort: 443,
				Protocol:     "TCP",
			},
			existingListener: &entityv2.GlobalListener{
				ID:       "lis-456",
				Name:     "listener-port-443",
				Port:     443,
				Protocol: "TCP",
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				// No updates needed
			},
			wantName: "listener-port-443",
			wantId:   "lis-456",
			wantPort: 443,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
			tt.setupMock(mockVngcloudRepo)

			cfg := &config.Config{
				GlobalLoadBalancerOpts: config.GlobalLoadBalancerOpts{
					DefaultAllowedCidrs:      "0.0.0.0/0",
					DefaultTimeoutClient:     50,
					DefaultTimeoutMember:     50,
					DefaultTimeoutConnection: 5,
				},
			}

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: mockVngcloudRepo,
				cfg:          cfg,
				lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
					Status: v1alpha1.GlobalLoadBalancerConfigStatus{},
				},
			}

			currentListeners := &entityv2.ListGlobalListeners{
				Items: []*entityv2.GlobalListener{tt.existingListener},
			}

			result, err := task.deployListener(
				context.Background(),
				"glb-123",
				tt.listenerSpec,
				currentListeners,
				[]v1alpha1.CreatedGlobalPool{},
			)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.wantName, result.Name, "Name must be populated from API entity (BUG-04)")
			assert.Equal(t, tt.wantId, result.Id)
			assert.Equal(t, tt.wantPort, result.Port)

			mockVngcloudRepo.AssertExpectations(t)
		})
	}
}
