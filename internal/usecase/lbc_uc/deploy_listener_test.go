package lbc_uc

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

func TestDefaultModelDeployTask_DeployListeners_Success(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50000,
			DefaultTimeoutMember:     5000,
			DefaultTimeoutConnection: 5000,
		},
	}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	lbc := &v1alpha1.LoadBalancerConfig{
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:            "test-listener-1",
					Protocol:        loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort:    80,
					DefaultPoolName: ptr.To("test-pool-1"),
				},
				{
					Name:            "test-listener-2",
					Protocol:        loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort:    443,
					DefaultPoolName: ptr.To("test-pool-2"),
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     lbc,
	}

	mapPoolNameToID := map[string]string{
		"test-pool-1": "pool-123",
		"test-pool-2": "pool-456",
	}

	// Mock empty current listeners (need to create new)
	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-123").
		Return(&entity.ListListeners{Items: []*entity.Listener{}}, nil)

	// Mock listener creation for port 80
	createdListener1 := &entity.Listener{
		UUID:         "listener-123",
		Name:         "test-listener-1",
		ProtocolPort: 80,
	}
	mockVngcloudRepo.EXPECT().
		CreateListener(mock.Anything, "lb-123", mock.AnythingOfType("*v2.CreateListenerRequest")).
		Return(createdListener1, nil).Once()

	// Mock wait for LB active after first listener
	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil).Once()

	// Mock listener creation for port 443
	createdListener2 := &entity.Listener{
		UUID:         "listener-456",
		Name:         "test-listener-2",
		ProtocolPort: 443,
	}
	mockVngcloudRepo.EXPECT().
		CreateListener(mock.Anything, "lb-123", mock.AnythingOfType("*v2.CreateListenerRequest")).
		Return(createdListener2, nil).Once()

	// Mock wait for LB active after second listener
	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil).Once()

	mapListenerPortToID, err := task.deployListeners(context.Background(), "lb-123", mapPoolNameToID)
	assert.NoError(t, err)

	expectedMap := map[int]string{
		80:  "listener-123",
		443: "listener-456",
	}
	assert.Equal(t, expectedMap, mapListenerPortToID)
}

func TestDefaultModelDeployTask_DeployListener_CreateNew(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50000,
			DefaultTimeoutMember:     5000,
			DefaultTimeoutConnection: 5000,
		},
	}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	listenerSpec := v1alpha1.Listener{
		Name:              "new-listener",
		Protocol:          loadbalancerv2.ListenerProtocolHTTP,
		ProtocolPort:      8080,
		DefaultPoolName:   ptr.To("test-pool"),
		AllowedCidrs:      ptr.To("10.0.0.0/8"),
		TimeoutClient:     ptr.To(int32(60000)),
		TimeoutMember:     ptr.To(int32(6000)),
		TimeoutConnection: ptr.To(int32(6000)),
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	mapPoolNameToID := map[string]string{
		"test-pool": "pool-789",
	}

	currentListeners := &entity.ListListeners{Items: []*entity.Listener{}}

	createdListener := &entity.Listener{
		UUID:         "new-listener-123",
		Name:         "new-listener",
		ProtocolPort: 8080,
	}
	mockVngcloudRepo.EXPECT().
		CreateListener(mock.Anything, "lb-123", mock.AnythingOfType("*v2.CreateListenerRequest")).
		Run(func(ctx context.Context, lbID string, req loadbalancerv2.ICreateListenerRequest) {
			// Verify that the request contains our custom values
			createReq := req.(*loadbalancerv2.CreateListenerRequest)
			assert.Equal(t, "new-listener", createReq.ListenerName)
			assert.Equal(t, loadbalancerv2.ListenerProtocolHTTP, createReq.ListenerProtocol)
			assert.Equal(t, 8080, createReq.ListenerProtocolPort)
			assert.Equal(t, "10.0.0.0/8", createReq.AllowedCidrs)
			assert.Equal(t, 60000, createReq.TimeoutClient)
			assert.Equal(t, 6000, createReq.TimeoutMember)
			assert.Equal(t, 6000, createReq.TimeoutConnection)
			assert.NotNil(t, createReq.DefaultPoolId)
			assert.Equal(t, "pool-789", *createReq.DefaultPoolId)
		}).
		Return(createdListener, nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	listenerID, err := task.deployListener(context.Background(), "lb-123", listenerSpec, currentListeners, mapPoolNameToID)
	assert.NoError(t, err)
	assert.Equal(t, "new-listener-123", listenerID)
}

func TestDefaultModelDeployTask_DeployListener_UpdateExisting(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50000,
			DefaultTimeoutMember:     5000,
			DefaultTimeoutConnection: 5000,
		},
	}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	listenerSpec := v1alpha1.Listener{
		Name:              "existing-listener",
		Protocol:          loadbalancerv2.ListenerProtocolHTTP,
		ProtocolPort:      8080,
		DefaultPoolName:   ptr.To("new-pool"),       // Different from current
		AllowedCidrs:      ptr.To("192.168.0.0/16"), // Different from current
		TimeoutClient:     ptr.To(int32(70000)),     // Different from current
		TimeoutMember:     ptr.To(int32(7000)),      // Different from current
		TimeoutConnection: ptr.To(int32(7000)),      // Different from current
	}

	currentListener := &entity.Listener{
		UUID:              "existing-listener-123",
		Name:              "existing-listener",
		Protocol:          "HTTP",
		ProtocolPort:      8080,
		DefaultPoolId:     "old-pool",
		DefaultPoolName:   "old-pool",
		AllowedCidrs:      "0.0.0.0/0",
		TimeoutClient:     50000,
		TimeoutMember:     5000,
		TimeoutConnection: 5000,
	}

	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{currentListener},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	mapPoolNameToID := map[string]string{
		"new-pool": "pool-new-123",
		"old-pool": "pool-old-123",
	}

	// Mock update listener (all fields changed)
	mockVngcloudRepo.EXPECT().
		UpdateListener(mock.Anything, "lb-123", "existing-listener-123", mock.AnythingOfType("*v2.UpdateListenerRequest")).
		Run(func(ctx context.Context, lbID, listenerID string, req loadbalancerv2.IUpdateListenerRequest) {
			updateReq := req.(*loadbalancerv2.UpdateListenerRequest)
			assert.Equal(t, "192.168.0.0/16", updateReq.AllowedCidrs)
			assert.Equal(t, 70000, updateReq.TimeoutClient)
			assert.Equal(t, 7000, updateReq.TimeoutMember)
			assert.Equal(t, 7000, updateReq.TimeoutConnection)
			assert.Equal(t, "pool-new-123", updateReq.DefaultPoolId)
		}).
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	listenerID, err := task.deployListener(context.Background(), "lb-123", listenerSpec, currentListeners, mapPoolNameToID)
	assert.NoError(t, err)
	assert.Equal(t, "existing-listener-123", listenerID)
}

func TestDefaultModelDeployTask_DeployListener_ProtocolMismatch(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	listenerSpec := v1alpha1.Listener{
		Name:         "existing-listener",
		Protocol:     loadbalancerv2.ListenerProtocolHTTP, // Different from current
		ProtocolPort: 8080,
	}

	currentListener := &entity.Listener{
		UUID:         "existing-listener-123",
		Name:         "existing-listener",
		Protocol:     "TCP", // Different from desired
		ProtocolPort: 8080,
	}

	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{currentListener},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	mapPoolNameToID := map[string]string{}

	listenerID, err := task.deployListener(context.Background(), "lb-123", listenerSpec, currentListeners, mapPoolNameToID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "protocol mismatch")
	assert.Empty(t, listenerID)
}

func TestDefaultModelDeployTask_DeployListener_NoUpdate(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50000,
			DefaultTimeoutMember:     5000,
			DefaultTimeoutConnection: 5000,
		},
	}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	listenerSpec := v1alpha1.Listener{
		Name:              "existing-listener",
		Protocol:          loadbalancerv2.ListenerProtocolHTTP,
		ProtocolPort:      8080,
		DefaultPoolName:   ptr.To("same-pool"),  // Same as current
		AllowedCidrs:      ptr.To("0.0.0.0/0"),  // Same as current
		TimeoutClient:     ptr.To(int32(50000)), // Same as current
		TimeoutMember:     ptr.To(int32(5000)),  // Same as current
		TimeoutConnection: ptr.To(int32(5000)),  // Same as current
	}

	currentListener := &entity.Listener{
		UUID:              "existing-listener-123",
		Name:              "existing-listener",
		Protocol:          "HTTP",
		ProtocolPort:      8080,
		DefaultPoolId:     "pool-same-123",
		DefaultPoolName:   "same-pool",
		AllowedCidrs:      "0.0.0.0/0",
		TimeoutClient:     50000,
		TimeoutMember:     5000,
		TimeoutConnection: 5000,
	}

	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{currentListener},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	mapPoolNameToID := map[string]string{
		"same-pool": "pool-same-123",
	}

	// No UpdateListener call should be made since nothing changed
	// No WaitForLBActive call should be made since no update was performed

	listenerID, err := task.deployListener(context.Background(), "lb-123", listenerSpec, currentListeners, mapPoolNameToID)
	assert.NoError(t, err)
	assert.Equal(t, "existing-listener-123", listenerID)
}

func TestDefaultModelDeployTask_BuildCreateListenerRequest(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultAllowedCidrs:      "0.0.0.0/0",
			DefaultTimeoutClient:     50000,
			DefaultTimeoutMember:     5000,
			DefaultTimeoutConnection: 5000,
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		cfg:      cfg,
		lbConfig: &v1alpha1.LoadBalancerConfig{},
	}

	tests := []struct {
		name             string
		listenerSpec     v1alpha1.Listener
		mapPoolNameToID  map[string]string
		expectedDefaults bool
		expectedCustom   bool
		expectedPoolID   string
	}{
		{
			name: "Use default values",
			listenerSpec: v1alpha1.Listener{
				Name:         "test-listener",
				Protocol:     loadbalancerv2.ListenerProtocolHTTP,
				ProtocolPort: 80,
			},
			mapPoolNameToID:  map[string]string{},
			expectedDefaults: true,
		},
		{
			name: "Use custom values",
			listenerSpec: v1alpha1.Listener{
				Name:              "custom-listener",
				Protocol:          loadbalancerv2.ListenerProtocolHTTPS,
				ProtocolPort:      443,
				DefaultPoolName:   ptr.To("custom-pool"),
				AllowedCidrs:      ptr.To("10.0.0.0/8"),
				TimeoutClient:     ptr.To(int32(60000)),
				TimeoutMember:     ptr.To(int32(6000)),
				TimeoutConnection: ptr.To(int32(6000)),
			},
			mapPoolNameToID: map[string]string{
				"custom-pool": "pool-custom-123",
			},
			expectedCustom: true,
			expectedPoolID: "pool-custom-123",
		},
		{
			name: "Pool not found in map",
			listenerSpec: v1alpha1.Listener{
				Name:            "test-listener",
				Protocol:        loadbalancerv2.ListenerProtocolHTTP,
				ProtocolPort:    80,
				DefaultPoolName: ptr.To("missing-pool"),
			},
			mapPoolNameToID:  map[string]string{},
			expectedDefaults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := task.buildCreateListenerRequest(context.Background(), "lb-123", tt.listenerSpec, tt.mapPoolNameToID)

			createReq := req.(*loadbalancerv2.CreateListenerRequest)
			assert.Equal(t, tt.listenerSpec.Name, createReq.ListenerName)
			assert.Equal(t, tt.listenerSpec.Protocol, createReq.ListenerProtocol)
			assert.Equal(t, int(tt.listenerSpec.ProtocolPort), createReq.ListenerProtocolPort)
			assert.Equal(t, "lb-123", createReq.LoadBalancerId)

			if tt.expectedDefaults {
				assert.Equal(t, cfg.LoadBalancerOpts.DefaultAllowedCidrs, createReq.AllowedCidrs)
				assert.Equal(t, cfg.LoadBalancerOpts.DefaultTimeoutClient, createReq.TimeoutClient)
				assert.Equal(t, cfg.LoadBalancerOpts.DefaultTimeoutMember, createReq.TimeoutMember)
				assert.Equal(t, cfg.LoadBalancerOpts.DefaultTimeoutConnection, createReq.TimeoutConnection)
			}

			if tt.expectedCustom {
				assert.Equal(t, "10.0.0.0/8", createReq.AllowedCidrs)
				assert.Equal(t, 60000, createReq.TimeoutClient)
				assert.Equal(t, 6000, createReq.TimeoutMember)
				assert.Equal(t, 6000, createReq.TimeoutConnection)
			}

			if tt.expectedPoolID != "" {
				assert.NotNil(t, createReq.DefaultPoolId)
				assert.Equal(t, tt.expectedPoolID, *createReq.DefaultPoolId)
			}
		})
	}
}

func TestDefaultModelDeployTask_DeployDeleteRedundantListeners(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	status := v1alpha1.LoadBalancerConfigStatus{
		CreatedListeners: []v1alpha1.CreatedListener{
			{Id: "listener-1"},
			{Id: "listener-2"},
			{Id: "listener-3"}, // This listener will be deleted (not in use)
		},
	}

	mapListenerPortToID := map[int]string{
		80:  "listener-1", // In use
		443: "listener-2", // In use
		// listener-3 is not in use and should be deleted
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	// Mock current listeners (all listeners exist)
	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{
			{UUID: "listener-1", ProtocolPort: 80},
			{UUID: "listener-2", ProtocolPort: 443},
			{UUID: "listener-3", ProtocolPort: 8080},
		},
	}

	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-123").
		Return(currentListeners, nil)

	// listener-3 should be deleted (not in use)
	mockVngcloudRepo.EXPECT().
		DeleteListener(mock.Anything, "lb-123", "listener-3").
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(&entity.LoadBalancer{UUID: "lb-123"}, nil)

	err := task.deployDeleteRedundantListeners(context.Background(), "lb-123", mapListenerPortToID, status)
	assert.NoError(t, err)
}

func TestDefaultModelDeployTask_DeployDeleteRedundantListeners_ListenerNotFound(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockK8sRepository(t)
	mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

	status := v1alpha1.LoadBalancerConfigStatus{
		CreatedListeners: []v1alpha1.CreatedListener{
			{Id: "listener-missing"}, // This listener doesn't exist anymore
		},
	}

	mapListenerPortToID := map[int]string{
		// No listeners in use
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		lbConfig:     &v1alpha1.LoadBalancerConfig{},
	}

	// Mock current listeners (listener-missing doesn't exist)
	currentListeners := &entity.ListListeners{
		Items: []*entity.Listener{},
	}

	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-123").
		Return(currentListeners, nil)

	// No DeleteListener call should be made since the listener doesn't exist
	// No WaitForLBActive call should be made since no deletion was performed

	err := task.deployDeleteRedundantListeners(context.Background(), "lb-123", mapListenerPortToID, status)
	assert.NoError(t, err)
}
