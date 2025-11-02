package vlbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

func TestNewVLBCUseCase(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	assert.NotNil(t, uc)
	assert.IsType(t, &vlbcUseCase{}, uc)
}

func TestVLBCUseCase_Init(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	err := uc.Init(context.Background())
	assert.NoError(t, err)
}

func TestVLBCUseCase_Delete(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-vlbc",
		},
	}

	err := uc.Delete(context.Background(), req)
	assert.NoError(t, err)
}

func TestVLBCUseCase_Ensure_VLBCNotFound(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-vlbc",
		},
	}

	// Mock GetVLBC to return NotFound error - using proper Kubernetes NotFound error
	notFoundErr := apierrors.NewNotFound(schema.GroupResource{
		Group:    "vngcloud.vn",
		Resource: "vngcloudloadbalancerconfigs",
	}, "test-vlbc")

	mockK8sRepo.EXPECT().
		GetVLBC(mock.Anything, req.NamespacedName).
		Return(nil, notFoundErr)

	err := uc.Ensure(context.Background(), req)
	assert.NoError(t, err) // Should ignore NotFound errors
}

func TestVLBCUseCase_Ensure_Success(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
			DefaultAllowedCidrs:       "0.0.0.0/0",
			DefaultTimeoutClient:      50000,
			DefaultTimeoutMember:      5000,
			DefaultTimeoutConnection:  5000,
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-vlbc",
		},
	}

	// Create a test VLBC
	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			LoadBalancerId:   ptr.To("lb-12345"),
			LoadBalancerName: "test-lb",
			PackageId:        ptr.To("package-1"),
			Pools:            []v1alpha1.Pool{},
			Listeners:        []v1alpha1.Listener{},
		},
		Status: v1alpha1.VngcloudLoadBalancerConfigStatus{
			LoadBalancerId: ptr.To("lb-12345"),
			Address:        ptr.To("10.0.0.1"),
		},
	}

	// Mock load balancer entity
	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-12345",
		Name:      "test-lb",
		Address:   "10.0.0.1",
		PackageID: "package-1",
	}

	// Set up expectations
	mockK8sRepo.EXPECT().
		GetVLBC(mock.Anything, req.NamespacedName).
		Return(vlbc, nil)

	mockVngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, "lb-12345").
		Return(loadBalancer, nil)

	mockVngcloudRepo.EXPECT().
		ListPool(mock.Anything, "lb-12345").
		Return(&entity.ListPools{Items: []*entity.Pool{}}, nil)

	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-12345").
		Return(&entity.ListListeners{Items: []*entity.Listener{}}, nil)

	mockK8sRepo.EXPECT().
		PatchMutateStatusVLBC(mock.Anything, vlbc, mock.AnythingOfType("func(context.Context, *v1alpha1.VngcloudLoadBalancerConfig)")).
		Return(nil)

	err := uc.Ensure(context.Background(), req)
	assert.NoError(t, err)
}

func TestVLBCUseCase_Ensure_LoadBalancerByName(t *testing.T) {
	cfg := &config.Config{
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,
			DefaultPoolAlgorithm:      "ROUND_ROBIN",
			DefaultAllowedCidrs:       "0.0.0.0/0",
			DefaultTimeoutClient:      50000,
			DefaultTimeoutMember:      5000,
			DefaultTimeoutConnection:  5000,
		},
	}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	uc := NewVLBCUseCase(cfg, mockK8sRepo, mockVngcloudRepo)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-vlbc",
		},
	}

	// Create a test VLBC with only LoadBalancerName specified
	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			LoadBalancerName: "test-lb-by-name",
			Pools:            []v1alpha1.Pool{},
			Listeners:        []v1alpha1.Listener{},
		},
		Status: v1alpha1.VngcloudLoadBalancerConfigStatus{},
	}

	// Mock load balancer entity
	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-67890",
		Name:      "test-lb-by-name",
		Address:   "10.0.0.2",
		PackageID: "package-1",
	}

	// Set up expectations
	mockK8sRepo.EXPECT().
		GetVLBC(mock.Anything, req.NamespacedName).
		Return(vlbc, nil)

	mockVngcloudRepo.EXPECT().
		GetLoadBalancerByName(mock.Anything, "test-lb-by-name").
		Return(loadBalancer, nil)

	mockVngcloudRepo.EXPECT().
		ListPool(mock.Anything, "lb-67890").
		Return(&entity.ListPools{Items: []*entity.Pool{}}, nil)

	mockVngcloudRepo.EXPECT().
		ListListenerOfLB(mock.Anything, "lb-67890").
		Return(&entity.ListListeners{Items: []*entity.Listener{}}, nil)

	mockK8sRepo.EXPECT().
		PatchMutateStatusVLBC(mock.Anything, vlbc, mock.AnythingOfType("func(context.Context, *v1alpha1.VngcloudLoadBalancerConfig)")).
		Return(nil)

	err := uc.Ensure(context.Background(), req)
	assert.NoError(t, err)
}

func TestDefaultModelDeployTask_DeployLoadBalancer_ExistingLBID(t *testing.T) {
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
			LoadBalancerId: ptr.To("lb-existing"),
		},
		Status: v1alpha1.VngcloudLoadBalancerConfigStatus{
			LoadBalancerId: ptr.To("lb-existing"),
		},
	}

	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-existing",
		Name:      "existing-lb",
		Address:   "10.0.0.3",
		PackageID: "package-1",
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	mockVngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, "lb-existing").
		Return(loadBalancer, nil)

	mockK8sRepo.EXPECT().
		PatchMutateStatusVLBC(mock.Anything, vlbc, mock.AnythingOfType("func(context.Context, *v1alpha1.VngcloudLoadBalancerConfig)")).
		Return(nil)

	lbID, err := task.deployLoadBalancer(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "lb-existing", lbID)
}

func TestDefaultModelDeployTask_DeployLoadBalancer_Migration(t *testing.T) {
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
			LoadBalancerId: ptr.To("lb-new"),
		},
		Status: v1alpha1.VngcloudLoadBalancerConfigStatus{
			LoadBalancerId: ptr.To("lb-old"),
		},
	}

	newLoadBalancer := &entity.LoadBalancer{
		UUID:      "lb-new",
		Name:      "new-lb",
		Address:   "10.0.0.4",
		PackageID: "package-1",
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	mockVngcloudRepo.EXPECT().
		GetLoadBalancerByID(mock.Anything, "lb-new").
		Return(newLoadBalancer, nil)

	mockK8sRepo.EXPECT().
		PatchMutateStatusVLBC(mock.Anything, vlbc, mock.AnythingOfType("func(context.Context, *v1alpha1.VngcloudLoadBalancerConfig)")).
		Return(nil)

	lbID, err := task.deployLoadBalancer(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "lb-new", lbID)
}

func TestDefaultModelDeployTask_DeployPackageId_NoPackageUpdate(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			PackageId: ptr.To("package-1"),
		},
	}

	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-123",
		PackageID: "package-1", // Same package ID
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	err := task.deployPackageId(context.Background(), loadBalancer)
	assert.NoError(t, err)
}

func TestDefaultModelDeployTask_DeployPackageId_NeedResize(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			PackageId: ptr.To("package-2"),
		},
	}

	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-123",
		PackageID: "package-1", // Different package ID
	}

	resizedLoadBalancer := &entity.LoadBalancer{
		UUID:      "lb-123",
		PackageID: "package-2",
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	mockVngcloudRepo.EXPECT().
		ResizeLoadBalancer(mock.Anything, "lb-123", "package-2").
		Return(nil)

	mockVngcloudRepo.EXPECT().
		WaitForLBActive(mock.Anything, "lb-123").
		Return(resizedLoadBalancer, nil)

	err := task.deployPackageId(context.Background(), loadBalancer)
	assert.NoError(t, err)
}

func TestDefaultModelDeployTask_DeployPackageId_EmptyPackageID(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			// No PackageID specified
		},
	}

	loadBalancer := &entity.LoadBalancer{
		UUID:      "lb-123",
		PackageID: "package-1",
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	err := task.deployPackageId(context.Background(), loadBalancer)
	assert.NoError(t, err) // Should not do anything if PackageID is not specified
}

func TestDefaultModelDeployTask_DeployPackageId_NilLoadBalancer(t *testing.T) {
	cfg := &config.Config{}
	mockK8sRepo := repository.NewMockIK8sRepository(t)
	mockVngcloudRepo := repository.NewMockIVngCloudRepository(t)

	vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
		Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
			PackageId: ptr.To("package-2"),
		},
	}

	task := &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		cfg:          cfg,
		vngcloudRepo: mockVngcloudRepo,
		k8sRepo:      mockK8sRepo,
		vlbConfig:    vlbc,
	}

	err := task.deployPackageId(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load balancer entity is nil")
}
