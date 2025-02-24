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

package controller

import (
	"context"
	"fmt"
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
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	mockIngressReconciler *IngressReconciler
	mockServiceReconciler *ServiceReconciler
	mockVGLBReconciler    *VngcloudGlobalLoadBalancerReconciler
	mockProvider          *provider.MockProvider

	mockConfig = &config.Config{
		Cluster: struct {
			ClusterName string "mapstructure:\"clusterName\""
			ClusterID   string "mapstructure:\"clusterID\""
			Region      string "mapstructure:\"region\""
		}{ClusterName: "test-cluster", ClusterID: mockClusterID},
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

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,

		// The BinaryAssetsDirectory is only required if you want to run the tests directly
		// without call the makefile target test. If not informed it will look for the
		// default path defined in controller-runtime which is /usr/local/kubebuilder/.
		// Note that you must have the required binaries setup under the bin directory to perform
		// the tests directly. When we run make test it will be setup and used automatically.
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.31.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = networkingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = v1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	ctx, cancel = context.WithCancel(context.TODO())
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	finalizerManager := k8s.NewDefaultFinalizerManager(k8sManager.GetClient(), ctrl.Log)
	mockProvider = provider.NewMockProvider()
	updateTracker := NewUpdateTracker(mockProvider)
	mockIngressReconciler = &IngressReconciler{
		modeTest: true,
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: k8sManager.GetEventRecorderFor("vngcloud-load-balancer-controller"),

		Config:              mockConfig,
		Provider:            mockProvider,
		FinalizerManager:    finalizerManager,
		timeReconcilePeriod: 2 * time.Second,
		UpdateTracker:       updateTracker,
	}
	err = mockIngressReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	mockServiceReconciler = &ServiceReconciler{
		modeTest: true,
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: k8sManager.GetEventRecorderFor("vngcloud-load-balancer-controller"),

		Config:              mockConfig,
		Provider:            mockProvider,
		FinalizerManager:    finalizerManager,
		timeReconcilePeriod: 2 * time.Second,
		UpdateTracker:       updateTracker,
	}
	err = mockServiceReconciler.SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	mockVGLBReconciler = &VngcloudGlobalLoadBalancerReconciler{
		// modeTest: true,
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Recorder: k8sManager.GetEventRecorderFor("vngcloud-load-balancer-controller"),

		Config:           mockConfig,
		Provider:         mockProvider,
		FinalizerManager: finalizerManager,
		// timeReconcilePeriod: 2 * time.Second,
		// UpdateTracker:       updateTracker,
	}
	err = mockVGLBReconciler.SetupWithManager(k8sManager)
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
						NodeName: PointerOf("test"),
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
						AppProtocol: PointerOf("http"),
					},
				},
			},
		},
	}
}
