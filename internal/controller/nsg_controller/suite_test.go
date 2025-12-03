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

package nsg_controller

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/anngdinh/operator-helper/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/k8s_repo"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/nsg_uc"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/nsg"
)

var (
	ctx               context.Context
	cancel            context.CancelFunc
	testEnv           *envtest.Environment
	cfg               *rest.Config
	k8sClient         client.Client
	mockNSGReconciler *NodeSecurityGroupReconciler
	vngcloudRepo      *vngcloud_mocks.MockProvider

	mockConfig = &config.Config{
		Cluster: struct {
			IsRunRemote bool   `mapstructure:"isRunRemote"`
			Namespace   string `mapstructure:"namespace"`
			ClusterID   string `mapstructure:"clusterID"`
			Region      string `mapstructure:"region"`
		}{IsRunRemote: false, ClusterID: mockClusterID},
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultL4PackageName: vngcloud_mocks.MockL4PackageName,
			DefaultPoolAlgorithm: "ROUND_ROBIN",
			DefaultScheme:        "Internal",

			DefaultTimeoutClient:     50,
			DefaultTimeoutConnection: 5,
			DefaultTimeoutMember:     50,

			DefaultHealthyThreshold:   3,
			DefaultUnhealthyThreshold: 3,
			DefaultInterval:           30,
			DefaultTimeout:            5,

			DefaultAllowedCidrs: "0.0.0.0/0",
		},
	}
)

const (
	mockClusterID = "k8s-00000000-0000-0000-0000-000000000000"
)

func TestNSGController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeSecurityGroup Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
			fileName := path.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
			return "", fileName
		},
	})

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Setup manager and reconcilers
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	finalizerManager := k8s.NewDefaultFinalizerManager(k8sManager.GetClient(), ctrl.Log)
	k8sRepo := k8s_repo.NewK8sRepository(k8sManager.GetClient())

	// Setup mock VNG Cloud repository
	vngcloudRepo = vngcloud_mocks.NewMockProvider()
	err = vngcloudRepo.Init(nil)
	Expect(err).NotTo(HaveOccurred())

	// Setup NSG reconciler
	nsgUseCase := nsg_uc.NewNodeSecurityGroupUseCase(
		mockConfig,
		k8sRepo,
		vngcloudRepo,
	)
	reconcileCounters := metricsutil.NewReconcileCounters()
	lbcMetricsCollector := lbcmetrics.NewCollector(metrics.Registry, k8sManager, reconcileCounters, ctrl.Log.WithName("controller_metrics"))
	mockNSGReconciler = NewNodeSecurityGroupReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		nsgUseCase,
		k8sManager.GetEventRecorderFor("nsg-controller"),
		finalizerManager,
		nsg.NewNodeSecurityGroupUtils(domain.NsgFinalizer),
		lbcMetricsCollector,
		reconcileCounters,
		1,
	)
	err = mockNSGReconciler.SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// Start the manager
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Create mock nodes
	err = k8sClient.Create(ctx, vngcloud_mocks.MockNode1)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, vngcloud_mocks.MockNode2)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, vngcloud_mocks.MockNode3)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, vngcloud_mocks.MockNode4)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

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
