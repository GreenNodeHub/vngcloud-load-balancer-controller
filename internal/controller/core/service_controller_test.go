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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lbcmetrics "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/lbc"
	metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/shared_constants"
)

// MockServiceController is a mock implementation of ServiceController
type MockServiceController struct {
	mock.Mock
}

func (m *MockServiceController) Ensure(ctx context.Context, req ctrl.Request) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockServiceController) Delete(ctx context.Context, req ctrl.Request) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// MockServiceUtils is a mock implementation of ServiceUtils
type MockServiceUtils struct {
	mock.Mock
}

func (m *MockServiceUtils) IsServiceSupported(svc *corev1.Service) bool {
	args := m.Called(svc)
	return args.Bool(0)
}

func (m *MockServiceUtils) IsServicePendingFinalization(svc *corev1.Service) bool {
	args := m.Called(svc)
	return args.Bool(0)
}

var _ = Describe("Service Controller", func() {
	var (
		reconciler       *ServiceReconciler
		mockController   *MockServiceController
		mockServiceUtils *MockServiceUtils
		fakeClient       client.Client
		scheme           *runtime.Scheme
		ctx              context.Context
		testNamespace    string
		testServiceName  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testNamespace = "test-namespace"
		testServiceName = "test-service"

		// Setup scheme
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)

		// Create fake client
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		// Initialize mocks
		mockController = new(MockServiceController)
		mockServiceUtils = new(MockServiceUtils)

		// Initialize reconciler
		reconciler = &ServiceReconciler{
			Client:            fakeClient,
			Scheme:            scheme,
			ServiceController: mockController,
			serviceUtils:      mockServiceUtils,
			eventRecorder:     &record.FakeRecorder{},
			logger:            zap.New(zap.UseDevMode(true)),
			reconcileCounters: metricsutil.NewReconcileCounters(),
			metricsCollector:  lbcmetrics.NewMockCollector(),
		}
	})

	Context("When reconciling a LoadBalancer Service", func() {
		var testService *corev1.Service

		BeforeEach(func() {
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
		})

		It("should successfully reconcile a new LoadBalancer service", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Ensure", mock.Anything, req)
		})

		It("should handle service deletion with finalizer", func() {
			// Create service with finalizer
			testService.Finalizers = []string{shared_constants.ServiceFinalizer}
			now := metav1.Now()
			testService.DeletionTimestamp = &now
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(false)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(true)
			mockController.On("Delete", mock.Anything, mock.Anything).Return(nil)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Delete", mock.Anything, req)
		})

		It("should ignore non-LoadBalancer services without finalizer", func() {
			// Create ClusterIP service
			testService.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(false)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertNotCalled(GinkgoT(), "Ensure", mock.Anything, mock.Anything)
			mockController.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
		})

		It("should handle service type change from LoadBalancer to ClusterIP with finalizer", func() {
			// Create ClusterIP service with finalizer (was LoadBalancer before)
			testService.Spec.Type = corev1.ServiceTypeClusterIP
			testService.Finalizers = []string{shared_constants.ServiceFinalizer}
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(false)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(true)
			mockController.On("Delete", mock.Anything, mock.Anything).Return(nil)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Delete", mock.Anything, req)
		})

		It("should handle service not found error gracefully", func() {
			// Don't create the service - simulate it being deleted
			// The fake client might still call Get which returns an empty service
			// So we need to set up mocks for the empty service case
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(false).Maybe()
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false).Maybe()

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify - should not return error for NotFound
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertNotCalled(GinkgoT(), "Ensure", mock.Anything, mock.Anything)
			mockController.AssertNotCalled(GinkgoT(), "Delete", mock.Anything, mock.Anything)
		})

		It("should handle controller ensure error with requeue", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			genericError := errors.New("some transient error")
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(genericError)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify - generic errors should be returned for requeue
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("some transient error"))
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Ensure", mock.Anything, req)
		})

		It("should handle controller delete error", func() {
			// Create service with finalizer and deletion timestamp
			testService.Finalizers = []string{shared_constants.ServiceFinalizer}
			now := metav1.Now()
			testService.DeletionTimestamp = &now
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(false)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(true)
			deleteError := errors.New("failed to delete load balancer")
			mockController.On("Delete", mock.Anything, mock.Anything).Return(deleteError)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete load balancer"))
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Delete", mock.Anything, req)
		})
	})

	Context("When service annotations or spec change", func() {
		var testService *corev1.Service

		BeforeEach(func() {
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"service.beta.kubernetes.io/vngcloud-load-balancer-id": "lb-123",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
				},
			}
		})

		It("should reconcile when annotations change", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil).Once()

			// First reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Update annotations
			var updatedService corev1.Service
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      testServiceName,
				Namespace: testNamespace,
			}, &updatedService)).To(Succeed())

			updatedService.Annotations["service.beta.kubernetes.io/vngcloud-load-balancer-scheme"] = "internal"
			Expect(fakeClient.Update(ctx, &updatedService)).To(Succeed())

			// Setup mocks for second reconcile
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil).Once()

			// Second reconcile after annotation change
			result, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify Ensure was called twice
			mockController.AssertNumberOfCalls(GinkgoT(), "Ensure", 2)
		})

		It("should reconcile when service ports change", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil).Once()

			// First reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Update service ports
			var updatedService corev1.Service
			Expect(fakeClient.Get(ctx, types.NamespacedName{
				Name:      testServiceName,
				Namespace: testNamespace,
			}, &updatedService)).To(Succeed())

			updatedService.Spec.Ports = append(updatedService.Spec.Ports, corev1.ServicePort{
				Name:     "https",
				Port:     443,
				Protocol: corev1.ProtocolTCP,
			})
			Expect(fakeClient.Update(ctx, &updatedService)).To(Succeed())

			// Setup mocks for second reconcile
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil).Once()

			// Second reconcile after spec change
			result, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify Ensure was called twice
			mockController.AssertNumberOfCalls(GinkgoT(), "Ensure", 2)
		})
	})

	Context("When handling concurrent reconciles", func() {
		It("should handle multiple services independently", func() {
			// Create multiple services
			service1 := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-1",
					Namespace: testNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: 80}},
				},
			}
			service2 := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-2",
					Namespace: testNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type:  corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			}

			Expect(fakeClient.Create(ctx, service1)).To(Succeed())
			Expect(fakeClient.Create(ctx, service2)).To(Succeed())

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(nil)

			// Reconcile both services
			req1 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "service-1",
					Namespace: testNamespace,
				},
			}
			req2 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "service-2",
					Namespace: testNamespace,
				},
			}

			result1, err1 := reconciler.Reconcile(ctx, req1)
			result2, err2 := reconciler.Reconcile(ctx, req2)

			// Verify both reconciles succeeded
			Expect(err1).NotTo(HaveOccurred())
			Expect(result1).To(Equal(ctrl.Result{}))
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2).To(Equal(ctrl.Result{}))

			// Verify Ensure was called for both
			mockController.AssertNumberOfCalls(GinkgoT(), "Ensure", 2)
		})
	})

	Context("Edge cases and error scenarios", func() {
		var testService *corev1.Service

		BeforeEach(func() {
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
				},
			}
		})

		It("should handle context cancellation", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Create a cancelled context
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()

			// Setup mocks
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Return(context.Canceled)

			// Reconcile with cancelled context
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}

			// Note: We use the regular context here because the reconciler creates its own timeout context
			result, err := reconciler.Reconcile(cancelledCtx, req)

			// Verify context cancellation is handled
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should handle timeout", func() {
			// Create the service
			Expect(fakeClient.Create(ctx, testService)).To(Succeed())

			// Setup mocks with a long-running operation
			mockServiceUtils.On("IsServiceSupported", mock.Anything).Return(true)
			mockServiceUtils.On("IsServicePendingFinalization", mock.Anything).Return(false)
			mockController.On("Ensure", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				// Simulate a long-running operation
				time.Sleep(100 * time.Millisecond)
			}).Return(nil)

			// Reconcile
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testServiceName,
					Namespace: testNamespace,
				},
			}
			result, err := reconciler.Reconcile(ctx, req)

			// Verify
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			mockController.AssertCalled(GinkgoT(), "Ensure", mock.Anything, req)
		})
	})
})
