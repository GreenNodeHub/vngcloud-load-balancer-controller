package glbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// TestDeployPool_PopulatesCreatedPoolMembers tests that when deployPool creates a new pool,
// the returned CreatedGlobalPool.CreatedPoolMembers is populated from the ListGlobalPoolMembers
// API response (STAT-01).
func TestDeployPool_PopulatesCreatedPoolMembers(t *testing.T) {
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
	mockK8sRepo := repository.NewMockK8sRepository(t)

	cfg := &config.Config{
		GlobalLoadBalancerOpts: config.GlobalLoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		cfg:          cfg,
		lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
			Status: v1alpha1.GlobalLoadBalancerConfigStatus{},
		},
	}

	poolSpec := &v1alpha1.GlobalPool{
		Name:     "test-pool",
		Protocol: "TCP",
		HealthMonitor: v1alpha1.GlobalPoolHealthMonitor{
			Protocol: global.GlobalPoolHealthCheckProtocolTCP,
		},
		PoolMembers: []v1alpha1.GlobalPoolMember{
			{Name: "pm-region-1"},
		},
	}

	currentPools := &entityv2.ListGlobalPools{
		Items: []*entityv2.GlobalPool{}, // empty — forces create path
	}

	weight := 1
	monitorPort := 80

	// Setup mocks
	mockVngcloudRepo.On("CreateGlobalPool", mock.Anything, "glb-123", mock.Anything).
		Return(&entityv2.GlobalPool{ID: "pool-123", Name: "test-pool"}, nil)

	mockVngcloudRepo.On("WaitGlobalLoadBalancerActive", mock.Anything, "glb-123").
		Return(&entityv2.GlobalLoadBalancer{}, nil)

	mockVngcloudRepo.On("ListGlobalPoolMembers", mock.Anything, "glb-123", "pool-123").
		Return(&entityv2.ListGlobalPoolMembers{
			Items: []*entityv2.GlobalPoolMember{
				{
					ID:   "pm-1",
					Name: "pm-region-1",
					Members: &entityv2.ListGlobalMembers{
						Items: []*entityv2.GlobalPoolMemberDetail{
							{
								Name:        "m1",
								Address:     "10.0.0.1",
								Port:        80,
								Weight:      weight,
								MonitorPort: monitorPort,
								SubnetID:    "subnet-1",
							},
						},
					},
				},
			},
		}, nil)

	mockK8sRepo.On("PatchMutateStatusGlobalLoadBalancerConfig", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	result, err := task.deployPool(context.Background(), "glb-123", poolSpec, currentPools)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.CreatedPoolMembers, 1, "CreatedPoolMembers must be populated from ListGlobalPoolMembers API response")
	assert.Equal(t, "pm-1", result.CreatedPoolMembers[0].Id)
	assert.Equal(t, "10.0.0.1", result.CreatedPoolMembers[0].CreatedMembers[0].Address)
}

// TestDeployPool_StatusUpdatedOnCreate tests that statusUpdatePoolMember (via
// PatchMutateStatusGlobalLoadBalancerConfig) is called during pool creation (STAT-01).
func TestDeployPool_StatusUpdatedOnCreate(t *testing.T) {
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
	mockK8sRepo := repository.NewMockK8sRepository(t)

	cfg := &config.Config{
		GlobalLoadBalancerOpts: config.GlobalLoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		cfg:          cfg,
		lbConfig: &v1alpha1.GlobalLoadBalancerConfig{
			Status: v1alpha1.GlobalLoadBalancerConfigStatus{},
		},
	}

	poolSpec := &v1alpha1.GlobalPool{
		Name:     "test-pool",
		Protocol: "TCP",
		HealthMonitor: v1alpha1.GlobalPoolHealthMonitor{
			Protocol: global.GlobalPoolHealthCheckProtocolTCP,
		},
		PoolMembers: []v1alpha1.GlobalPoolMember{
			{Name: "pm-region-1"},
		},
	}

	currentPools := &entityv2.ListGlobalPools{
		Items: []*entityv2.GlobalPool{},
	}

	weight := 1
	monitorPort := 80

	mockVngcloudRepo.On("CreateGlobalPool", mock.Anything, "glb-123", mock.Anything).
		Return(&entityv2.GlobalPool{ID: "pool-123", Name: "test-pool"}, nil)

	mockVngcloudRepo.On("WaitGlobalLoadBalancerActive", mock.Anything, "glb-123").
		Return(&entityv2.GlobalLoadBalancer{}, nil)

	mockVngcloudRepo.On("ListGlobalPoolMembers", mock.Anything, "glb-123", "pool-123").
		Return(&entityv2.ListGlobalPoolMembers{
			Items: []*entityv2.GlobalPoolMember{
				{
					ID:   "pm-1",
					Name: "pm-region-1",
					Members: &entityv2.ListGlobalMembers{
						Items: []*entityv2.GlobalPoolMemberDetail{
							{
								Name:        "m1",
								Address:     "10.0.0.1",
								Port:        80,
								Weight:      weight,
								MonitorPort: monitorPort,
								SubnetID:    "subnet-1",
							},
						},
					},
				},
			},
		}, nil)

	mockK8sRepo.On("PatchMutateStatusGlobalLoadBalancerConfig", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)

	_, err := task.deployPool(context.Background(), "glb-123", poolSpec, currentPools)

	assert.NoError(t, err)
	mockK8sRepo.AssertCalled(t, "PatchMutateStatusGlobalLoadBalancerConfig", mock.Anything, mock.Anything, mock.Anything)
	mockK8sRepo.AssertExpectations(t)
}
