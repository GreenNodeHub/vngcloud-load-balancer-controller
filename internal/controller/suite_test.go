/*
Copyright 2025 annd2.

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

package controller

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/anngdinh/operator-helper/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/core"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/k8s_repo"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/lbc_uc"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/nsg_uc"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/service_uc"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/lbc"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/nsg"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/service"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg                   *rest.Config
	k8sClient             client.Client
	testEnv               *envtest.Environment
	ctx                   context.Context
	cancel                context.CancelFunc
	mockServiceReconciler *core.ServiceReconciler
	mockLBCReconciler     *LoadBalancerConfigReconciler
	mockNSGReconciler     *NodeSecurityGroupReconciler
	vngcloudRepo          *vngcloud_mocks.MockProvider
	cniDetector           *utils.MockCniDetector

	mockConfig = &config.Config{
		Cluster: struct {
			IsRunRemote bool   `mapstructure:"isRunRemote"` // run from another cluster, watch through clusterAPI
			Namespace   string `mapstructure:"namespace"`   // if run remote, the namespace of cluster
			ClusterID   string `mapstructure:"clusterID"`   // clusterID of cluster
			Region      string `mapstructure:"region"`      // region of cluster
		}{IsRunRemote: false, ClusterID: mockClusterID},
		LoadBalancerOpts: config.LoadBalancerOpts{
			DefaultL4PackageName: "NLB_Small",
			DefaultPoolAlgorithm: "ROUND_ROBIN",

			DefaultTimeoutClient:     50,
			DefaultTimeoutConnection: 5,
			DefaultTimeoutMember:     50,
		},
	}

	mockNode1 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-1",
			Labels: map[string]string{
				"nodeName":                  "mock-node-1",
				"nodeGroup":                 "mock-node-group-a",
				"vks.vngcloud.vn/mgmt-zone": "mock-mgmt-zone",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://ins-00000000-0000-0000-0000-000000000001",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
				{Type: corev1.NodeHostName, Address: "mock-node-1"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	mockNode2 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-2",
			Labels: map[string]string{
				"nodeName":  "mock-node-2",
				"nodeGroup": "mock-node-group-a",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://ins-00000000-0000-0000-0000-000000000002",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.2"},
				{Type: corev1.NodeHostName, Address: "mock-node-2"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	mockNode3 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-3",
			Labels: map[string]string{
				"nodeName":  "mock-node-3",
				"nodeGroup": "mock-node-group-b",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://ins-00000000-0000-0000-0000-000000000003",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.3"},
				{Type: corev1.NodeHostName, Address: "mock-node-3"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}

	mockNode4 = &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "mock-node-4",
			Labels: map[string]string{
				"nodeName":  "mock-node-4",
				"nodeGroup": "mock-node-group-b",
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: "vngcloud://ins-00000000-0000-0000-0000-000000000004",
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.4"},
				{Type: corev1.NodeHostName, Address: "mock-node-4"},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
)

const (
	timeout       = time.Second * 5
	duration      = time.Second * 10
	interval      = time.Millisecond * 250
	mockClusterID = "k8s-00000000-0000-0000-0000-000000000000"
)

var (
	timeWaitRecocile = 2 * time.Second
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
			fileName := path.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
			// return frame.Function, fileName
			return "", fileName
		},
	})

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = networkingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
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

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	finalizerManager := k8s.NewDefaultFinalizerManager(k8sManager.GetClient(), ctrl.Log)
	k8sRepo := k8s_repo.NewK8sRepository(k8sManager.GetClient())
	// vngcloudRepo, err := vngcloud_repo.NewVngCloudRepository(ctx, mockConfig)
	vngcloudRepo = vngcloud_mocks.NewMockProvider()
	err = vngcloudRepo.Init(nil)
	Expect(err).NotTo(HaveOccurred())

	annotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX) // TODO: change prefix if needed
	cniDetector = new(utils.MockCniDetector)
	cniDetector.EXPECT().DetectCNIType(mock.Anything).Return(utils.CiliumNativeRouting, nil)
	endpointResolver := utils.NewDefaultEndpointResolver(ctx, k8sManager.GetClient())
	serviceUtils := service.NewServiceUtils(consts.ServiceFinalizer)
	serviceUseCase := service_uc.NewServiceUseCase(
		mockClusterID, k8sRepo, vngcloudRepo, annotationParser, serviceUtils, cniDetector, endpointResolver)
	mockServiceReconciler = core.NewServiceReconciler(
		serviceUseCase,
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		finalizerManager,
		k8sManager.GetEventRecorderFor("service-controller"),
		serviceUtils,
	)
	err = mockServiceReconciler.SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	lbcUseCase := lbc_uc.NewLoadBalancerConfigUseCase(
		mockConfig,
		k8sRepo,
		vngcloudRepo,
	)
	mockLBCReconciler = NewLoadBalancerConfigReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		lbcUseCase,
		k8sManager.GetEventRecorderFor("lbc-controller"),
		finalizerManager,
		lbc.NewLoadBalancerConfigUtils(consts.LBCFinalizer),
	)
	err = mockLBCReconciler.SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	nsgUseCase := nsg_uc.NewNodeSecurityGroupUseCase(
		mockConfig,
		k8sRepo,
		vngcloudRepo,
	)
	mockNSGReconciler = NewNodeSecurityGroupReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		nsgUseCase,
		k8sManager.GetEventRecorderFor("nsg-controller"),
		finalizerManager,
		nsg.NewNodeSecurityGroupUtils(consts.NSGFinalizer),
	)
	err = mockNSGReconciler.SetupWithManager(ctx, k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	// Create mock node
	err = k8sClient.Create(ctx, mockNode1)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, mockNode2)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, mockNode3)
	Expect(err).ToNot(HaveOccurred())
	err = k8sClient.Create(ctx, mockNode4)
	Expect(err).ToNot(HaveOccurred())

	// // comment these line to make the test run faster
	// mockProvider.WaitAfterTime = 3 * time.Second
	// timeWaitRecocile = 20 * time.Second
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func printEndTest() {
	logrus.Info("======================================================")
	logrus.Info("======================================================")
	logrus.Info()
}

var _ = newEndpointResource("placeholder", "placeholder")

func newEndpointResource(name, namespace string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP:       "172.172.172.0",
						Hostname: "test",
						NodeName: ptr.To("test"),
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: namespace,
							Name:      "pod-1",
						},
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Name:        "http",
						Port:        80,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr.To("http"),
					},
				},
			},
		},
	}
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
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
