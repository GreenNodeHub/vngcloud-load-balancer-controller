/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package core

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anngdinh/operator-helper/k8s"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

// Shared manager and mock for service controller tests
var (
	testMgr            ctrl.Manager
	testMockController *mockServiceController
	testMgrContext     context.Context
	testMgrCancel      context.CancelFunc
	setupOnce          sync.Once
)

func setupSharedManager() {
	setupOnce.Do(func() {
		// Create mock service controller
		testMockController = &mockServiceController{}

		// Create manager
		var err error
		testMgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(),
		})
		Expect(err).ToNot(HaveOccurred())

		// Create ServiceReconciler with the new architecture
		reconciler := &ServiceReconciler{
			Client:                  testMgr.GetClient(),
			Scheme:                  testMgr.GetScheme(),
			ServiceController:       testMockController,
			FinalizerManager:        k8s.NewDefaultFinalizerManager(testMgr.GetClient(), logr.Discard()),
			serviceUtils:            service.NewServiceUtils(consts.ServiceFinalizer),
			eventRecorder:           &record.FakeRecorder{},
			logger:                  logr.Discard(),
			maxConcurrentReconciles: 1,
		}

		// Setup controller with manager
		err = reconciler.SetupWithManager(ctx, testMgr)
		Expect(err).ToNot(HaveOccurred())

		// Start manager in background
		testMgrContext, testMgrCancel = context.WithCancel(ctx)
		go func() {
			defer GinkgoRecover()
			err := testMgr.Start(testMgrContext)
			Expect(err).ToNot(HaveOccurred())
		}()
	})
}

// Mock implementations for service controller testing
type mockServiceController struct {
	mu          sync.Mutex
	ensureCalls int
	deleteCalls int
}

func (m *mockServiceController) Ensure(ctx context.Context, req ctrl.Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCalls++
	return nil
}

func (m *mockServiceController) Delete(ctx context.Context, req ctrl.Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls++
	return nil
}

func (m *mockServiceController) GetEnsureCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureCalls
}

func (m *mockServiceController) GetDeleteCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteCalls
}

func (m *mockServiceController) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCalls = 0
	m.deleteCalls = 0
}

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Setup shared manager for service controller tests
	setupSharedManager()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	// Stop the shared manager first
	if testMgrCancel != nil {
		testMgrCancel()
	}

	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
