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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// These unit tests just test the controller logic, not the real k8s cluster
// Eg: create a service -> k8s auto create endpoint -> k8s update this endpoint
// 		-> controller should reconcile twice when create service and when update endpoint (ignore create endpoint)
// 		-> but in test, it just reconcile once when create service

var _ = Describe("Service Controller", func() {
	Context("When create, update or delete a service", func() {
		It("should successfully reconcile the resource", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Service: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.ensureTest = funcTest
			mockServiceReconciler.deleteTest = funcTest

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newSeviceResource("test-service", "default")
			Expect(service).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(1))

			// when update a service LoadBalancer type to the other type, the controller will reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			// when delete a service ClusterIP type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			// when create a service ClusterIP type, the controller will not reconcile it
			service = newSeviceResource("test-service-2", "default")
			Expect(service).NotTo(BeNil())
			service.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			// when update a service to LoadBalancer type, the controller will reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-2", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeLoadBalancer
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(3))

			// when delete a service LoadBalancer type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-2", Namespace: "default"}, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(4))
		})
	})

	Context("When create, update or delete a endpoint", func() {
		It("should successfully reconcile the resource", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Service: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.ensureTest = funcTest
			mockServiceReconciler.deleteTest = funcTest

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newSeviceResource("test-service", "default")
			Expect(service).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(1))

			// when create a endpoint for the service, the controller will not reconcile it
			endpoint := newEndpointResource("test-service", "default")
			Expect(endpoint).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(1))

			// when update a endpoint for the service, the controller will reconcile it
			endpoint = &corev1.Endpoints{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, endpoint)).Should(Succeed())
			endpoint.Subsets[0].Addresses = append(endpoint.Subsets[0].Addresses, corev1.EndpointAddress{IP: "11.0.0.0"})
			Expect(k8sClient.Update(ctx, endpoint)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			// when delete a endpoint for the service, the controller will reconcile it
			endpoint = &corev1.Endpoints{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, endpoint)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(3))

			// when delete a service LoadBalancer type, the controller will reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(4))
		})
	})

	Context("When update Service LoadBalancer to NodePort", func() {
		It("should create a delete event", func() {
			countReconcile, countReconcileDelete := 0, 0
			mockServiceReconciler.ensureTest = func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Service: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.deleteTest = func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcileDelete++
				klog.Info("Delete Service: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			klog.Info("countReconcile: ", countReconcile, " countReconcileDelete: ", countReconcileDelete)

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newSeviceResource("test-service", "default")
			Expect(service).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				klog.Info("countReconcile: ", countReconcile, " countReconcileDelete: ", countReconcileDelete)
				return countReconcile == 1 && countReconcileDelete == 0
			}, timeout, interval).Should(Equal(true))

			// when update a service LoadBalancer type to the other type, the controller will reconcile it as a delete event
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeNodePort
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1 && countReconcileDelete == 1
			}, timeout, interval).Should(Equal(true))

			// when update a service not LoadBalancer type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Annotations = map[string]string{"test": "test"}
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1 && countReconcileDelete == 1
			}, timeout, interval).Should(Equal(true))

			// when update a service to LoadBalancer type, the controller will reconcile it as a create event
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeLoadBalancer
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 2 && countReconcileDelete == 1
			}, timeout, interval).Should(Equal(true))

			// when delete a service LoadBalancer type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 2 && countReconcileDelete == 2
			}, timeout, interval).Should(Equal(true))
		})
	})

	// Context("aaaaaaaaaaaaaaaa", func() {
	// 	It("aaaaaaaaaaaaaaaaaaaaaa", func() {
	// 	})
	// })

	// Context("aaaaaaaaaaaaaaaa", func() {
	// 	It("aaaaaaaaaaaaaaaaaaaaaa", func() {
	// 	})
	// })
})

func newSeviceResource(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(80)},
			},
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
}

func newEndpointResource(name, namespace string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{IP: "10.0.0.0"}},
				Ports: []corev1.EndpointPort{
					{Name: "http", Port: 80},
				},
			},
		},
	}
}
