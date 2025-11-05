package nsg_uc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/k8s_repo"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

func TestEnsureStatusNodeSecurityGroupIntegration(t *testing.T) {
	ctx := context.Background()

	// Setup envtest
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	// Set binary directory for envtest
	if binDir := getFirstFoundEnvTestBinaryDir(); binDir != "" {
		testEnv.BinaryAssetsDirectory = binDir
	}

	// Register scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Start test environment
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	defer func() {
		err := testEnv.Stop()
		assert.NoError(t, err)
	}()

	// Create k8s client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)

	// Setup repositories and use case
	k8sRepo := k8s_repo.NewK8sRepository(k8sClient)
	vngcloudRepo := vngcloud_mocks.NewMockProvider()
	err = vngcloudRepo.Init(nil)
	require.NoError(t, err)

	mockConfig := &config.Config{
		Cluster: struct {
			IsRunRemote bool   `mapstructure:"isRunRemote"`
			Namespace   string `mapstructure:"namespace"`
			ClusterID   string `mapstructure:"clusterID"`
			Region      string `mapstructure:"region"`
		}{IsRunRemote: false, ClusterID: "test-cluster"},
	}

	useCase := NewNodeSecurityGroupUseCase(mockConfig, k8sRepo, vngcloudRepo).(*nsgUseCase)

	t.Run("should add a single server to empty status", func(t *testing.T) {
		// Create test NSG
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nsg-1",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		// Test
		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", nil, []string{"sg-1", "sg-2"})
		assert.NoError(t, err)

		// Verify
		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)

		assert.Len(t, testNSG.Status.ServerSecurityGroups, 1)
		assert.Equal(t, "server-1", testNSG.Status.ServerSecurityGroups[0].ServerId)
		assert.Equal(t, []string{"sg-1", "sg-2"}, testNSG.Status.ServerSecurityGroups[0].AttachedSecurityGroupIds)
		assert.Nil(t, testNSG.Status.ServerSecurityGroups[0].Error)
	})

	t.Run("should record error when provided", func(t *testing.T) {
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nsg-2",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		// Test with error
		testErr := errors.New("attachment failed")
		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", testErr, []string{})
		assert.NoError(t, err)

		// Verify
		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)

		assert.Len(t, testNSG.Status.ServerSecurityGroups, 1)
		require.NotNil(t, testNSG.Status.ServerSecurityGroups[0].Error)
		assert.Equal(t, testErr.Error(), *testNSG.Status.ServerSecurityGroups[0].Error)
	})

	t.Run("should update existing server status without duplication", func(t *testing.T) {
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nsg-3",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		// Add initial status
		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", nil, []string{"sg-1"})
		require.NoError(t, err)

		// Get fresh copy and update
		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)

		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", nil, []string{"sg-2", "sg-3"})
		assert.NoError(t, err)

		// Verify no duplication
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)

		assert.Len(t, testNSG.Status.ServerSecurityGroups, 1)
		assert.Equal(t, []string{"sg-2", "sg-3"}, testNSG.Status.ServerSecurityGroups[0].AttachedSecurityGroupIds)
	})

	t.Run("should clear previous error on successful update", func(t *testing.T) {
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nsg-4",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		// Add status with error
		testErr := errors.New("previous error")
		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", testErr, []string{})
		require.NoError(t, err)

		// Get fresh copy
		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		require.NotNil(t, testNSG.Status.ServerSecurityGroups[0].Error)

		// Update without error
		err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, "server-1", nil, []string{"sg-1"})
		assert.NoError(t, err)

		// Verify error was cleared
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		assert.Nil(t, testNSG.Status.ServerSecurityGroups[0].Error)
		assert.Equal(t, []string{"sg-1"}, testNSG.Status.ServerSecurityGroups[0].AttachedSecurityGroupIds)
	})

	t.Run("should add 300 servers and store all of them", func(t *testing.T) {
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nsg-5",
				Namespace: "default",
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}

		// Add 300 servers sequentially
		for i := 1; i <= 300; i++ {
			serverID := fmt.Sprintf("server-%d", i)
			secgroups := []string{fmt.Sprintf("sg-%d", i)}

			// Add server
			err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, serverID, nil, secgroups)
			require.NoError(t, err)

			// Get fresh copy for next iteration
			err = k8sClient.Get(ctx, nsName, testNSG)
			require.NoError(t, err)

			// Verify count matches expected number of servers added so far
			assert.Len(t, testNSG.Status.ServerSecurityGroups, i, "After adding server %d, expected %d servers", i, i)
		}

		// Final verification - all 300 servers should be present
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		assert.Len(t, testNSG.Status.ServerSecurityGroups, 300)

		// Verify each server is present with correct data
		serverMap := make(map[string]v1alpha1.ServerSecurityGroupStatus)
		for _, status := range testNSG.Status.ServerSecurityGroups {
			serverMap[status.ServerId] = status
		}

		for i := 1; i <= 300; i++ {
			serverID := fmt.Sprintf("server-%d", i)
			status, exists := serverMap[serverID]
			assert.True(t, exists, "Server %s should exist", serverID)
			if exists {
				assert.Equal(t, []string{fmt.Sprintf("sg-%d", i)}, status.AttachedSecurityGroupIds)
				assert.Nil(t, status.Error)
			}
		}
	})

	t.Run("should continue updating status with DeletionTimestamp and finalizer", func(t *testing.T) {
		testNSG := &v1alpha1.NodeSecurityGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "test-nsg-6",
				Namespace:  "default",
				Finalizers: []string{"nsg.vngcloud.vn/resources"},
			},
			Spec: v1alpha1.NodeSecurityGroupSpec{
				SelectNodeLabels: map[string]string{"test": "true"},
			},
		}
		err := k8sClient.Create(ctx, testNSG)
		require.NoError(t, err)
		defer k8sClient.Delete(ctx, testNSG)

		nsName := types.NamespacedName{Name: testNSG.Name, Namespace: testNSG.Namespace}

		// Add first 50 servers before deletion
		for i := 1; i <= 50; i++ {
			serverID := fmt.Sprintf("server-%d", i)
			secgroups := []string{fmt.Sprintf("sg-%d", i)}

			err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, serverID, nil, secgroups)
			require.NoError(t, err)

			// Get fresh copy for next iteration
			err = k8sClient.Get(ctx, nsName, testNSG)
			require.NoError(t, err)
		}

		// Verify first 50 servers added
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		assert.Len(t, testNSG.Status.ServerSecurityGroups, 50)

		// Mark object for deletion (this sets DeletionTimestamp)
		err = k8sClient.Delete(ctx, testNSG)
		require.NoError(t, err)

		// Get fresh copy with DeletionTimestamp
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		require.NotNil(t, testNSG.DeletionTimestamp, "DeletionTimestamp should be set")
		t.Logf("✓ Object has DeletionTimestamp: %v", testNSG.DeletionTimestamp)

		// Continue adding servers even with DeletionTimestamp (simulating cleanup phase)
		for i := 51; i <= 150; i++ {
			serverID := fmt.Sprintf("server-%d", i)
			secgroups := []string{fmt.Sprintf("sg-%d", i)}

			err = useCase.ensureStatusNodeSecurityGroup(ctx, testNSG, serverID, nil, secgroups)
			require.NoError(t, err)

			// Get fresh copy for next iteration
			err = k8sClient.Get(ctx, nsName, testNSG)
			require.NoError(t, err)

			// Verify count increments correctly
			assert.Len(t, testNSG.Status.ServerSecurityGroups, i,
				"After adding server %d (with DeletionTimestamp), expected %d servers", i, i)
		}

		// Final verification - all 150 servers should be present even with DeletionTimestamp
		err = k8sClient.Get(ctx, nsName, testNSG)
		require.NoError(t, err)
		assert.Len(t, testNSG.Status.ServerSecurityGroups, 150)
		require.NotNil(t, testNSG.DeletionTimestamp, "DeletionTimestamp should still be set")

		// Verify each server is present with correct data
		serverMap := make(map[string]v1alpha1.ServerSecurityGroupStatus)
		for _, status := range testNSG.Status.ServerSecurityGroups {
			serverMap[status.ServerId] = status
		}

		for i := 1; i <= 150; i++ {
			serverID := fmt.Sprintf("server-%d", i)
			status, exists := serverMap[serverID]
			assert.True(t, exists, "Server %s should exist (even with DeletionTimestamp)", serverID)
			if exists {
				assert.Equal(t, []string{fmt.Sprintf("sg-%d", i)}, status.AttachedSecurityGroupIds)
				assert.Nil(t, status.Error)
			}
		}

		t.Logf("✓ Successfully updated status 150 times with DeletionTimestamp set")

		// Remove finalizer to allow actual deletion
		testNSG.Finalizers = []string{}
		err = k8sClient.Update(ctx, testNSG)
		require.NoError(t, err)
	})
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
