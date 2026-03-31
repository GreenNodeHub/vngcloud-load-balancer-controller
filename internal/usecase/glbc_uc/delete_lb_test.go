package glbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer verifies BUG-03:
// When the LB is found to be empty after cleanup, it must call DeleteGlobalLoadBalancer
// (not DeleteLoadBalancer) to correctly target the global load balancer API.
func TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*repository.MockVngCloudRepository)
		lbConfig  *v1alpha1.GlobalLoadBalancerConfig
	}{
		{
			name: "empty LB after cleanup must call DeleteGlobalLoadBalancer not DeleteLoadBalancer",
			setupMock: func(m *repository.MockVngCloudRepository) {
				// deleteLoadBalancer -> GetGlobalLoadBalancerByID: LB exists
				m.EXPECT().
					GetGlobalLoadBalancerByID(mock.Anything, "glb-123").
					Return(&entityv2.GlobalLoadBalancer{ID: "glb-123"}, nil).
					Once()

				// canDeleteWholeLoadBalancer -> ListGlobalListeners:
				// Return a foreign listener that is NOT in our status, so canDelete = false
				// This forces the else-branch (cleanup path)
				m.EXPECT().
					ListGlobalListeners(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalListeners{
						Items: []*entityv2.GlobalListener{
							{ID: "foreign-lis", Name: "foreign-listener"},
						},
					}, nil).
					Once()

				// deleteRedundantListeners -> ListGlobalListeners:
				// Status has no createdListeners, so deleteCandidates is empty — returns immediately
				m.EXPECT().
					ListGlobalListeners(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalListeners{Items: []*entityv2.GlobalListener{}}, nil).
					Once()

				// deleteRedundantPools -> ListGlobalPools:
				// Status has no createdPools, so deleteCandidates is empty — returns immediately
				m.EXPECT().
					ListGlobalPools(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalPools{Items: []*entityv2.GlobalPool{}}, nil).
					Once()

				// deleteRedundantPools -> ListGlobalListeners (to check pools in use)
				m.EXPECT().
					ListGlobalListeners(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalListeners{Items: []*entityv2.GlobalListener{}}, nil).
					Once()

				// isLoadBalancerEmpty -> ListGlobalListeners: empty
				m.EXPECT().
					ListGlobalListeners(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalListeners{Items: []*entityv2.GlobalListener{}}, nil).
					Once()

				// isLoadBalancerEmpty -> ListGlobalPools: empty
				m.EXPECT().
					ListGlobalPools(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalPools{Items: []*entityv2.GlobalPool{}}, nil).
					Once()

				// isEmpty == true -> must call DeleteGlobalLoadBalancer (BUG: currently calls DeleteLoadBalancer)
				m.EXPECT().
					DeleteGlobalLoadBalancer(mock.Anything, "glb-123").
					Return(nil).
					Once()
			},
			lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
				Status: v1alpha1.GlobalLoadBalancerConfigStatus{
					// No created listeners or pools in status
					CreatedListeners: []v1alpha1.CreatedGlobalListener{},
					CreatedPools:     []v1alpha1.CreatedGlobalPool{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
			tt.setupMock(mockVngcloudRepo)

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: mockVngcloudRepo,
				lbConfig:     tt.lbConfig,
			}

			err := task.deleteLoadBalancer(context.Background(), "glb-123")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			mockVngcloudRepo.AssertExpectations(t)
		})
	}
}

// TestDeleteGlobalPool_CallsDeleteGlobalPool verifies the companion bug:
// deleteRedundantPools must call DeleteGlobalPool (not DeletePool) when removing pools.
func TestDeleteGlobalPool_CallsDeleteGlobalPool(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*repository.MockVngCloudRepository)
		lbConfig  *v1alpha1.GlobalLoadBalancerConfig
	}{
		{
			name: "deleteRedundantPools calls DeleteGlobalPool not DeletePool",
			setupMock: func(m *repository.MockVngCloudRepository) {
				// ListGlobalPools: pool-1 exists
				m.EXPECT().
					ListGlobalPools(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalPools{
						Items: []*entityv2.GlobalPool{
							{ID: "pool-1", Name: "pool-1"},
						},
					}, nil).
					Once()

				// ListGlobalListeners: no listeners using the pool
				m.EXPECT().
					ListGlobalListeners(mock.Anything, "glb-123").
					Return(&entityv2.ListGlobalListeners{Items: []*entityv2.GlobalListener{}}, nil).
					Once()

				// canDeleteWholePool -> ListGlobalPoolMembers: no members -> can delete whole
				m.EXPECT().
					ListGlobalPoolMembers(mock.Anything, "glb-123", "pool-1").
					Return(&entityv2.ListGlobalPoolMembers{Items: []*entityv2.GlobalPoolMember{}}, nil).
					Once()

				// Must call DeleteGlobalPool (BUG: currently calls DeletePool)
				m.EXPECT().
					DeleteGlobalPool(mock.Anything, "glb-123", "pool-1").
					Return(nil).
					Once()

				// WaitGlobalLoadBalancerActive after deletion
				m.EXPECT().
					WaitGlobalLoadBalancerActive(mock.Anything, "glb-123").
					Return(&entityv2.GlobalLoadBalancer{}, nil).
					Once()
			},
			lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
				Status: v1alpha1.GlobalLoadBalancerConfigStatus{
					CreatedPools: []v1alpha1.CreatedGlobalPool{
						{
							Id:   "pool-1",
							Name: "pool-1",
							// No CreatedPoolMembers — all members in cloud were also not ours
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
			tt.setupMock(mockVngcloudRepo)

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: mockVngcloudRepo,
				lbConfig:     tt.lbConfig,
			}

			err := task.deleteRedundantPools(context.Background(), "glb-123", []v1alpha1.CreatedGlobalPool{})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			mockVngcloudRepo.AssertExpectations(t)
		})
	}
}
