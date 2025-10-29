package vlbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

func TestDefaultModelDeployTask_DeployPools_Success(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			Pools: []v1alpha1.Pool{
				{
					Name:     "test-pool-1",
					Protocol: loadbalancerv2.PoolProtocolTCP,
					Members: []v1alpha1.PoolMember{
						{
							Name:        "member-1",
							IP:          "10.0.0.1",
							Port:        8080,
							MonitorPort: 8080,
						},
					},
					HealthMonitor: v1alpha1.PoolHealthMonitor{
						Protocol: loadbalancerv2.HealthCheckProtocolTCP,
					},
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	// Mock empty current pools (need to create new)
	mockVngcloudRepo.EXPECT().
		ListPool(mock.Anything, "lb-123").
		Return(&entity.ListPools{Items: []*entity.Pool{}}, nil)

	// Mock pool creation
	createdPool := &entity.Pool{
		UUID: "pool-123",
		Name: "test-pool-1",
	}
	mockVngcloudRepo.EXPECT().
		CreatePool(mock.Anything, "lb-123", mock.AnythingOfType("*v2.CreatePoolRequest")).
		Return(createdPool, nil)

	// Mock wait for LB active
	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	mapPoolNameToID, err := task.deployPools(context.Background(), "lb-123")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{"test-pool-1": "pool-123"}, mapPoolNameToID)
}

func TestDefaultModelDeployTask_DeployPool_CreateNew(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	pool := &v1alpha1.Pool{
		Name:     "new-pool",
		Protocol: loadbalancerv2.PoolProtocolTCP,
		Members: []v1alpha1.PoolMember{
			{
				Name:        "member-1",
				IP:          "10.0.0.1",
				Port:        8080,
				MonitorPort: 8080,
			},
		},
		HealthMonitor: v1alpha1.PoolHealthMonitor{
			Protocol: loadbalancerv2.HealthCheckProtocolTCP,
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    &v1alpha1.VngcloudLoadBalancerConfig{},
	}

	currentPools := &entity.ListPools{Items: []*entity.Pool{}}

	createdPool := &entity.Pool{
		UUID: "new-pool-123",
		Name: "new-pool",
	}
	mockVngcloudRepo.EXPECT().
		CreatePool(mock.Anything, "lb-123", mock.AnythingOfType("*v2.CreatePoolRequest")).
		Return(createdPool, nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	poolID, err := task.deployPool(context.Background(), "lb-123", pool, currentPools)
	assert.NoError(t, err)
	assert.Equal(t, "new-pool-123", poolID)
}

func TestDefaultModelDeployTask_DeployPool_UpdateExisting(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	pool := &v1alpha1.Pool{
		Name:      "existing-pool",
		Protocol:  loadbalancerv2.PoolProtocolTCP,
		Algorithm: ptr.To(loadbalancerv2.PoolAlgorithmLeastConn), // Different from current
		Members: []v1alpha1.PoolMember{
			{
				Name:        "member-1",
				IP:          "10.0.0.1",
				Port:        8080,
				MonitorPort: 8080,
			},
		},
		HealthMonitor: v1alpha1.PoolHealthMonitor{
			Protocol:           loadbalancerv2.HealthCheckProtocolTCP,
			HealthyThreshold:   ptr.To(5), // Different from current
			UnhealthyThreshold: ptr.To(3),
			Interval:           ptr.To(30),
			Timeout:            ptr.To(5),
		},
	}

	currentPool := &entity.Pool{
		UUID:              "existing-pool-123",
		Name:              "existing-pool",
		LoadBalanceMethod: "ROUND_ROBIN", // Different from desired
	}

	currentHealthMonitor := &entity.HealthMonitor{
		HealthyThreshold:    3, // Different from desired
		UnhealthyThreshold:  3,
		Interval:            30,
		Timeout:             5,
		HealthCheckProtocol: "TCP",
		HealthCheckPath:     nil,
		DomainName:          nil,
		SuccessCode:         nil,
		HealthCheckMethod:   nil,
		HttpVersion:         nil,
	}

	currentPools := &entity.ListPools{
		Items: []*entity.Pool{currentPool},
	}

	currentMembers := &entity.ListMembers{
		Items: []*entity.Member{
			{
				Address:      "10.0.0.1",
				ProtocolPort: 8080,
				MonitorPort:  8080,
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    &v1alpha1.VngcloudLoadBalancerConfig{},
	}

	// Mock get health monitor
	mockVngcloudRepo.EXPECT().
		GetPoolHealthMonitorById(mock.Anything, "lb-123", "existing-pool-123").
		Return(currentHealthMonitor, nil)

	// Mock update pool (algorithm and healthy threshold changed)
	mockVngcloudRepo.EXPECT().
		UpdatePool(mock.Anything, "lb-123", "existing-pool-123", mock.AnythingOfType("*v2.UpdatePoolRequest")).
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	// Mock get pool members
	mockVngcloudRepo.EXPECT().
		GetPoolMembers(mock.Anything, "lb-123", "existing-pool-123").
		Return(currentMembers, nil)

	poolID, err := task.deployPool(context.Background(), "lb-123", pool, currentPools)
	assert.NoError(t, err)
	assert.Equal(t, "existing-pool-123", poolID)
}

func TestDefaultModelDeployTask_DeployPool_UpdateMembers(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	pool := &v1alpha1.Pool{
		Name:     "existing-pool",
		Protocol: loadbalancerv2.PoolProtocolTCP,
		Members: []v1alpha1.PoolMember{
			{
				Name:        "member-1",
				IP:          "10.0.0.1",
				Port:        8080,
				MonitorPort: 8080,
			},
			{
				Name:        "member-2", // New member
				IP:          "10.0.0.2",
				Port:        8080,
				MonitorPort: 8080,
			},
		},
		HealthMonitor: v1alpha1.PoolHealthMonitor{
			Protocol: loadbalancerv2.HealthCheckProtocolTCP,
		},
	}

	currentPool := &entity.Pool{
		UUID:              "existing-pool-123",
		Name:              "existing-pool",
		LoadBalanceMethod: "ROUND_ROBIN",
	}

	currentHealthMonitor := &entity.HealthMonitor{
		HealthyThreshold:    3,
		UnhealthyThreshold:  3,
		Interval:            30,
		Timeout:             5,
		HealthCheckProtocol: "TCP",
		HealthCheckPath:     nil,
		DomainName:          nil,
		SuccessCode:         nil,
		HealthCheckMethod:   nil,
		HttpVersion:         nil,
	}

	currentPools := &entity.ListPools{
		Items: []*entity.Pool{currentPool},
	}

	// Current members - only one member, different from desired
	currentMembers := &entity.ListMembers{
		Items: []*entity.Member{
			{
				Address:      "10.0.0.1",
				ProtocolPort: 8080,
				MonitorPort:  8080,
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    &v1alpha1.VngcloudLoadBalancerConfig{},
	}

	// Mock get health monitor
	mockVngcloudRepo.EXPECT().
		GetPoolHealthMonitorById(mock.Anything, "lb-123", "existing-pool-123").
		Return(currentHealthMonitor, nil)

	// Mock get pool members
	mockVngcloudRepo.EXPECT().
		GetPoolMembers(mock.Anything, "lb-123", "existing-pool-123").
		Return(currentMembers, nil)

	// Mock update pool members (members changed)
	mockVngcloudRepo.EXPECT().
		UpdatePoolMembers(mock.Anything, "lb-123", "existing-pool-123", mock.AnythingOfType("*v2.UpdatePoolMembersRequest")).
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	poolID, err := task.deployPool(context.Background(), "lb-123", pool, currentPools)
	assert.NoError(t, err)
	assert.Equal(t, "existing-pool-123", poolID)
}

func TestDefaultModelDeployTask_ComparePoolMembers(t *testing.T) {
	task := &defaultModelDeployTask{}

	tests := []struct {
		name           string
		poolMembers    []v1alpha1.PoolMember
		currentMembers *entity.ListMembers
		expectedResult bool
	}{
		{
			name: "Members match",
			poolMembers: []v1alpha1.PoolMember{
				{
					IP:          "10.0.0.1",
					Port:        8080,
					MonitorPort: 8080,
				},
				{
					IP:          "10.0.0.2",
					Port:        8080,
					MonitorPort: 8080,
				},
			},
			currentMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{
						Address:      "10.0.0.1",
						ProtocolPort: 8080,
						MonitorPort:  8080,
					},
					{
						Address:      "10.0.0.2",
						ProtocolPort: 8080,
						MonitorPort:  8080,
					},
				},
			},
			expectedResult: true,
		},
		{
			name: "Different number of members",
			poolMembers: []v1alpha1.PoolMember{
				{
					IP:          "10.0.0.1",
					Port:        8080,
					MonitorPort: 8080,
				},
			},
			currentMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{
						Address:      "10.0.0.1",
						ProtocolPort: 8080,
						MonitorPort:  8080,
					},
					{
						Address:      "10.0.0.2",
						ProtocolPort: 8080,
						MonitorPort:  8080,
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "Member not found",
			poolMembers: []v1alpha1.PoolMember{
				{
					IP:          "10.0.0.3", // Different IP
					Port:        8080,
					MonitorPort: 8080,
				},
			},
			currentMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{
						Address:      "10.0.0.1",
						ProtocolPort: 8080,
						MonitorPort:  8080,
					},
				},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := task.comparePoolMembers(context.Background(), tt.poolMembers, tt.currentMembers)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestDefaultModelDeployTask_CheckIfPoolMemberExist(t *testing.T) {
	task := &defaultModelDeployTask{}

	currentMembers := &entity.ListMembers{
		Items: []*entity.Member{
			{
				Address:      "10.0.0.1",
				ProtocolPort: 8080,
				MonitorPort:  8080,
			},
			{
				Address:      "10.0.0.2",
				ProtocolPort: 9090,
				MonitorPort:  9090,
			},
		},
	}

	tests := []struct {
		name           string
		member         *v1alpha1.PoolMember
		expectedResult bool
	}{
		{
			name: "Member exists",
			member: &v1alpha1.PoolMember{
				IP:          "10.0.0.1",
				Port:        8080,
				MonitorPort: 8080,
			},
			expectedResult: true,
		},
		{
			name: "Member does not exist - different IP",
			member: &v1alpha1.PoolMember{
				IP:          "10.0.0.3",
				Port:        8080,
				MonitorPort: 8080,
			},
			expectedResult: false,
		},
		{
			name: "Member does not exist - different port",
			member: &v1alpha1.PoolMember{
				IP:          "10.0.0.1",
				Port:        8081,
				MonitorPort: 8081,
			},
			expectedResult: false,
		},
		{
			name: "Member does not exist - different monitor port",
			member: &v1alpha1.PoolMember{
				IP:          "10.0.0.1",
				Port:        8080,
				MonitorPort: 8081,
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := task.checkIfPoolMemberExist(currentMembers, tt.member)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestDefaultModelDeployTask_DeployDeleteRedundantPools(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	status := v1alpha1.VngcloudLoadBalancerConfigStatus{
		CreatedPools: []v1alpha1.CreatedPool{
			{Id: "pool-1"},
			{Id: "pool-2"},
			{Id: "pool-3"}, // This pool will be deleted (not in use)
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    &v1alpha1.VngcloudLoadBalancerConfig{},
	}

	// Mock current listeners (pool-1 and pool-2 are in use)
	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{
			{
				UUID:          "listener-1",
				DefaultPoolId: "pool-1",
			},
			{
				UUID:          "listener-2",
				DefaultPoolId: "pool-2",
			},
		},
	}

	// Mock current pools (all pools exist)
	currentPools := &entity.ListPools{
		Items: []*entity.Pool{
			{UUID: "pool-1", Name: "pool-1"},
			{UUID: "pool-2", Name: "pool-2"},
			{UUID: "pool-3", Name: "pool-3"},
		},
	}

	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-123").
		Return(currentListeners, nil)

	mockVngcloudRepo.EXPECT().
		ListPool(mock.Anything, "lb-123").
		Return(currentPools, nil)

	// pool-3 should be deleted (not in use)
	mockVngcloudRepo.EXPECT().
		DeletePool(mock.Anything, "lb-123", "pool-3").
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	err := task.deployDeleteRedundantPools(context.Background(), "lb-123", status)
	assert.NoError(t, err)
}
