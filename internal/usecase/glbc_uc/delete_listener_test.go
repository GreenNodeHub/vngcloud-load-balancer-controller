package glbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func TestCanDeleteWholeListener(t *testing.T) {
	tests := []struct {
		name            string
		listener        *entityv2.GlobalListener
		newCreatedPools []v1alpha1.CreatedGlobalPool
		createdPools    []v1alpha1.CreatedGlobalPool
		setupMock       func(*repository.MockVngCloudRepository)
		wantCanDelete   bool
		wantErr         bool
	}{
		{
			// listener.GlobalPoolID is empty -> returns (true, nil)
			name: "TestCanDeleteWholeListener_NoPool",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools:    []v1alpha1.CreatedGlobalPool{},
			setupMock:       func(m *repository.MockVngCloudRepository) {},
			wantCanDelete:   true,
			wantErr:         false,
		},
		{
			// listener's pool ID is in newCreatedPools -> returns (false, nil) — pool still in use
			name: "TestCanDeleteWholeListener_PoolInNewSpec",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-1",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{
				{Id: "pool-1"},
			},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{Id: "pool-1"},
			},
			setupMock:     func(m *repository.MockVngCloudRepository) {},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			// listener's pool ID is NOT in status.createdPools -> returns (false, nil) — not our pool
			name: "TestCanDeleteWholeListener_PoolNotInStatus",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-external",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{Id: "pool-1"}, // different pool
			},
			setupMock:     func(m *repository.MockVngCloudRepository) {},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			// listener's pool has 1 PoolMember group with 2 members, all appear in status by Address+Port -> returns (true, nil)
			name: "TestCanDeleteWholeListener_AllMembersOwned",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-1",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{
					Id:   "pool-1",
					Name: "pool-1",
					CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{
						{
							Id:   "pm-1",
							Name: "pm-group-1",
							CreatedMembers: []v1alpha1.GlobalMember{
								{Address: "10.0.0.1", Port: 8080},
								{Address: "10.0.0.2", Port: 8080},
							},
						},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListGlobalPoolMembers(context.Background(), "lb-1", "pool-1").
					Return(&entityv2.ListGlobalPoolMembers{
						Items: []*entityv2.GlobalPoolMember{
							{
								ID:   "pm-1",
								Name: "pm-group-1",
								Members: &entityv2.ListGlobalMembers{
									Items: []*entityv2.GlobalPoolMemberDetail{
										{Address: "10.0.0.1", Port: 8080},
										{Address: "10.0.0.2", Port: 8080},
									},
								},
							},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			// listener's pool has a PoolMember group whose Name is not in status -> returns (false, nil)
			name: "TestCanDeleteWholeListener_GroupNotOwned",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-1",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{
					Id:   "pool-1",
					Name: "pool-1",
					CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{
						{
							Id:   "pm-1",
							Name: "pm-group-owned",
						},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListGlobalPoolMembers(context.Background(), "lb-1", "pool-1").
					Return(&entityv2.ListGlobalPoolMembers{
						Items: []*entityv2.GlobalPoolMember{
							{
								ID:   "pm-external",
								Name: "pm-group-external", // not in status
								Members: &entityv2.ListGlobalMembers{
									Items: []*entityv2.GlobalPoolMemberDetail{
										{Address: "10.0.0.1", Port: 8080},
									},
								},
							},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			// listener's pool has a PoolMember group that IS in status, but one individual member's Address+Port is not in createdMembers -> returns (false, nil)
			name: "TestCanDeleteWholeListener_MemberNotOwned",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-1",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{
					Id:   "pool-1",
					Name: "pool-1",
					CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{
						{
							Id:   "pm-1",
							Name: "pm-group-1",
							CreatedMembers: []v1alpha1.GlobalMember{
								{Address: "10.0.0.1", Port: 8080},
								// 10.0.0.2:8080 is NOT in status
							},
						},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListGlobalPoolMembers(context.Background(), "lb-1", "pool-1").
					Return(&entityv2.ListGlobalPoolMembers{
						Items: []*entityv2.GlobalPoolMember{
							{
								ID:   "pm-1",
								Name: "pm-group-1",
								Members: &entityv2.ListGlobalMembers{
									Items: []*entityv2.GlobalPoolMemberDetail{
										{Address: "10.0.0.1", Port: 8080},
										{Address: "10.0.0.2", Port: 8080}, // extra member not in status
									},
								},
							},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			// pool exists in status but ListGlobalPoolMembers returns empty Items -> returns (true, nil) — no members means we own all (vacuously true)
			name: "TestCanDeleteWholeListener_EmptyPoolMembers",
			listener: &entityv2.GlobalListener{
				ID:           "listener-1",
				GlobalPoolID: "pool-1",
			},
			newCreatedPools: []v1alpha1.CreatedGlobalPool{},
			createdPools: []v1alpha1.CreatedGlobalPool{
				{
					Id:   "pool-1",
					Name: "pool-1",
					CreatedPoolMembers: []v1alpha1.CreatedGlobalPoolMember{},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListGlobalPoolMembers(context.Background(), "lb-1", "pool-1").
					Return(&entityv2.ListGlobalPoolMembers{
						Items: []*entityv2.GlobalPoolMember{},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
			tt.setupMock(mockVngcloudRepo)

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: mockVngcloudRepo,
				lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
					Status: v1alpha1.GlobalLoadBalancerConfigStatus{
						CreatedPools: tt.createdPools,
					},
				},
			}

			canDelete, err := task.canDeleteWholeListener(
				context.Background(),
				"lb-1",
				tt.listener,
				tt.newCreatedPools,
			)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCanDelete, canDelete, "canDelete mismatch for test: %s", tt.name)
		})
	}
}
