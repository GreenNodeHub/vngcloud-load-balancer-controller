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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Service Reconciler", func() {
	Context("When creating a LoadBalancer service with manager setup", func() {
		var (
			testService *corev1.Service
			namespace   string
		)

		BeforeEach(func() {
			namespace = "test-service-" + fmt.Sprintf("%d", time.Now().UnixNano())

			// Reset mock counters
			testMockController.Reset()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create test service
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: namespace,
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
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
		})

		AfterEach(func() {
			// Clean up namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			k8sClient.Delete(ctx, ns)
		})

		It("should call ServiceController.Ensure once when LoadBalancer service is created", func() {
			By("Creating a LoadBalancer service")
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting for automatic reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			By("Verifying ServiceController.Ensure was called exactly once")
			Expect(testMockController.GetEnsureCallCount()).To(Equal(1))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should call ServiceController.Ensure when LoadBalancer service is updated", func() {
			By("Creating a LoadBalancer service")
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting for initial reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			By("Updating the service with new annotations")
			Eventually(func() error {
				currentService := &corev1.Service{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService); err != nil {
					return err
				}

				if currentService.Annotations == nil {
					currentService.Annotations = make(map[string]string)
				}
				currentService.Annotations["test-update"] = "updated-value"

				return k8sClient.Update(ctx, currentService)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for update reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 2))

			By("Verifying ServiceController.Ensure was called multiple times")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 2))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should not call ServiceController methods for non-LoadBalancer services", func() {
			By("Creating a ClusterIP service")
			testService.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting and ensuring no reconciliation happens")
			Consistently(func() int {
				return testMockController.GetEnsureCallCount()
			}, 3*time.Second, 200*time.Millisecond).Should(Equal(0))

			By("Verifying no ServiceController methods were called")
			Expect(testMockController.GetEnsureCallCount()).To(Equal(0))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should not call ServiceController methods for NodePort services", func() {
			By("Creating a NodePort service")
			testService.Spec.Type = corev1.ServiceTypeNodePort
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting and ensuring no reconciliation happens")
			Consistently(func() int {
				return testMockController.GetEnsureCallCount()
			}, 3*time.Second, 200*time.Millisecond).Should(Equal(0))

			By("Verifying no ServiceController methods were called")
			Expect(testMockController.GetEnsureCallCount()).To(Equal(0))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should call ServiceController.Ensure when service type changes from ClusterIP to LoadBalancer", func() {
			By("Creating a ClusterIP service first")
			testService.Spec.Type = corev1.ServiceTypeClusterIP
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting and ensuring no reconciliation happens for ClusterIP")
			Consistently(func() int {
				return testMockController.GetEnsureCallCount()
			}, 3*time.Second, 200*time.Millisecond).Should(Equal(0))

			By("Updating service type to LoadBalancer")
			Eventually(func() error {
				currentService := &corev1.Service{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService); err != nil {
					return err
				}

				currentService.Spec.Type = corev1.ServiceTypeLoadBalancer
				return k8sClient.Update(ctx, currentService)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for service type change to trigger reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			By("Verifying ServiceController.Ensure was called after type change")
			Expect(testMockController.GetEnsureCallCount()).To(Equal(1))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))

			By("Verifying finalizer was added")
			currentService := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: testService.Name, Namespace: testService.Namespace,
			}, currentService)).To(Succeed())
			Expect(len(currentService.Finalizers)).To(BeNumerically(">", 0))
		})

		It("should call ServiceController.Delete when service type changes from LoadBalancer to ClusterIP", func() {
			By("Creating a LoadBalancer service")
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting for initial reconciliation and finalizer to be added")
			Eventually(func() bool {
				currentService := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService)
				if err != nil {
					return false
				}
				return testMockController.GetEnsureCallCount() >= 1 && len(currentService.Finalizers) > 0
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())

			// Reset counters after initial setup
			testMockController.Reset()

			By("Updating service type to ClusterIP")
			Eventually(func() error {
				currentService := &corev1.Service{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService); err != nil {
					return err
				}

				currentService.Spec.Type = corev1.ServiceTypeClusterIP
				return k8sClient.Update(ctx, currentService)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for service type change to trigger cleanup")
			Eventually(func() int {
				return testMockController.GetDeleteCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			By("Verifying ServiceController.Delete was called due to type change")
			Expect(testMockController.GetDeleteCallCount()).To(Equal(1))

			By("Verifying service finalizer is eventually removed")
			Eventually(func() bool {
				currentService := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService)
				if err != nil {
					return false
				}
				return len(currentService.Finalizers) == 0
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())
		})

		It("should call ServiceController.Delete when LoadBalancer service is deleted", func() {
			By("Creating a LoadBalancer service")
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			By("Waiting for initial reconciliation and finalizer to be added")
			Eventually(func() bool {
				currentService := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService)
				if err != nil {
					return false
				}
				// Check that both Ensure was called and finalizer was added
				return testMockController.GetEnsureCallCount() >= 1 && len(currentService.Finalizers) > 0
			}, 10*time.Second, 200*time.Millisecond).Should(BeTrue())

			By("Deleting the service")
			Expect(k8sClient.Delete(ctx, testService)).To(Succeed())

			By("Verifying service has deletion timestamp but still exists due to finalizer")
			Eventually(func() bool {
				currentService := &corev1.Service{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testService.Name, Namespace: testService.Namespace,
				}, currentService)
				if err != nil {
					return false // Service not found
				}
				// Service should exist with deletion timestamp and finalizer
				return !currentService.DeletionTimestamp.IsZero() && len(currentService.Finalizers) > 0
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

			By("Waiting for delete reconciliation")
			Eventually(func() int {
				return testMockController.GetDeleteCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			By("Verifying ServiceController.Delete was called")
			Expect(testMockController.GetDeleteCallCount()).To(Equal(1))
		})
	})

	Context("When endpoint events occur", func() {
		var (
			testService  *corev1.Service
			testEndpoint *corev1.Endpoints
			namespace    string
		)

		BeforeEach(func() {
			namespace = "test-endpoint-" + fmt.Sprintf("%d", time.Now().UnixNano())

			// Reset mock counters
			testMockController.Reset()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create test service first
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoint-test-service",
					Namespace: namespace,
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
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			// Wait for initial service reconciliation
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			// Reset counters after initial setup
			testMockController.Reset()

			// Create corresponding endpoint
			testEndpoint = &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoint-test-service", // Same name as service
					Namespace: namespace,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "10.0.0.1",
							},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "http",
								Port:     8080,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}
		})

		AfterEach(func() {
			// Clean up namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			k8sClient.Delete(ctx, ns)
		})

		It("should trigger service reconciliation when endpoint is created", func() {
			By("Creating an endpoint for the LoadBalancer service")
			Expect(k8sClient.Create(ctx, testEndpoint)).To(Succeed())

			By("Waiting for endpoint event to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to endpoint event")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should trigger service reconciliation when endpoint subsets change", func() {
			By("Creating an endpoint")
			Expect(k8sClient.Create(ctx, testEndpoint)).To(Succeed())

			By("Waiting for initial endpoint reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Updating endpoint subsets")
			Eventually(func() error {
				currentEndpoint := &corev1.Endpoints{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testEndpoint.Name, Namespace: testEndpoint.Namespace,
				}, currentEndpoint); err != nil {
					return err
				}

				// Add another address to trigger subset change
				currentEndpoint.Subsets[0].Addresses = append(currentEndpoint.Subsets[0].Addresses,
					corev1.EndpointAddress{IP: "10.0.0.2"})

				return k8sClient.Update(ctx, currentEndpoint)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for endpoint update to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to endpoint update")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
		})

		It("should trigger service reconciliation when endpoint is deleted", func() {
			By("Creating an endpoint")
			Expect(k8sClient.Create(ctx, testEndpoint)).To(Succeed())

			By("Waiting for initial endpoint reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Deleting the endpoint")
			Expect(k8sClient.Delete(ctx, testEndpoint)).To(Succeed())

			By("Waiting for endpoint deletion to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to endpoint deletion")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
		})

		It("should not trigger reconciliation for endpoints of non-LoadBalancer services", func() {
			By("Creating a ClusterIP service instead")
			clusterIPService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-service",
					Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
						},
					},
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, clusterIPService)).To(Succeed())

			By("Creating an endpoint for the ClusterIP service")
			clusterIPEndpoint := &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "clusterip-service",
					Namespace: namespace,
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{IP: "10.0.0.3"},
						},
						Ports: []corev1.EndpointPort{
							{
								Name:     "http",
								Port:     8080,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, clusterIPEndpoint)).To(Succeed())

			By("Ensuring no reconciliation happens for ClusterIP service")
			Consistently(func() int {
				return testMockController.GetEnsureCallCount()
			}, 3*time.Second, 200*time.Millisecond).Should(Equal(0))

			By("Verifying no ServiceController methods were called")
			Expect(testMockController.GetEnsureCallCount()).To(Equal(0))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})
	})

	Context("When node events occur", func() {
		var (
			testService *corev1.Service
			testNode    *corev1.Node
			namespace   string
		)

		BeforeEach(func() {
			namespace = "test-node-" + fmt.Sprintf("%d", time.Now().UnixNano())

			// Reset mock counters
			testMockController.Reset()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			// Create test service
			testService = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node-test-service",
					Namespace: namespace,
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
					Selector: map[string]string{
						"app": "test",
					},
				},
			}
			Expect(k8sClient.Create(ctx, testService)).To(Succeed())

			// Wait for initial service reconciliation
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(Equal(1))

			// Reset counters after initial setup
			testMockController.Reset()

			// Create test node
			testNode = &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-" + fmt.Sprintf("%d", time.Now().UnixNano()),
					Labels: map[string]string{
						"kubernetes.io/hostname": "test-node",
					},
				},
				Spec: corev1.NodeSpec{
					ProviderID: "test://test-node-123",
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{Type: corev1.NodeInternalIP, Address: "10.0.1.100"},
						{Type: corev1.NodeHostName, Address: "test-node"},
					},
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
		})

		AfterEach(func() {
			// Clean up test node
			k8sClient.Delete(ctx, testNode)

			// Clean up namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			k8sClient.Delete(ctx, ns)
		})

		It("should trigger service reconciliation when node is created", func() {
			By("Creating a new node")
			Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

			By("Waiting for node event to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to node creation")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})

		It("should trigger service reconciliation when node readiness changes", func() {
			By("Creating a node")
			Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

			By("Waiting for initial node reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Updating node to make it not ready")
			Eventually(func() error {
				currentNode := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testNode.Name,
				}, currentNode); err != nil {
					return err
				}

				// Change node to not ready
				for i, condition := range currentNode.Status.Conditions {
					if condition.Type == corev1.NodeReady {
						currentNode.Status.Conditions[i].Status = corev1.ConditionFalse
						break
					}
				}

				return k8sClient.Status().Update(ctx, currentNode)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for node update to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to node readiness change")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
		})

		It("should trigger service reconciliation when node addresses change", func() {
			By("Creating a node")
			Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

			By("Waiting for initial node reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Updating node addresses")
			Eventually(func() error {
				currentNode := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testNode.Name,
				}, currentNode); err != nil {
					return err
				}

				// Add a new address
				currentNode.Status.Addresses = append(currentNode.Status.Addresses,
					corev1.NodeAddress{Type: corev1.NodeExternalIP, Address: "203.0.113.1"})

				return k8sClient.Status().Update(ctx, currentNode)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Waiting for node address change to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to node address change")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
		})

		It("should trigger service reconciliation when node is deleted", func() {
			By("Creating a node")
			Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

			By("Waiting for initial node reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Deleting the node")
			Expect(k8sClient.Delete(ctx, testNode)).To(Succeed())

			By("Waiting for node deletion to trigger service reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController.Ensure was called due to node deletion")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
		})

		It("should trigger reconciliation when node annotation changes", func() {
			By("Creating a node")
			Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

			By("Waiting for initial node reconciliation")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			// Reset counter
			testMockController.Reset()

			By("Updating node annotation")
			Eventually(func() error {
				currentNode := &corev1.Node{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: testNode.Name,
				}, currentNode); err != nil {
					return err
				}

				// Add an annotation - this should trigger reconciliation
				// because all node updates now trigger reconciliation
				if currentNode.Annotations == nil {
					currentNode.Annotations = make(map[string]string)
				}
				currentNode.Annotations["test.example.com/annotation"] = "value"

				return k8sClient.Update(ctx, currentNode)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			By("Ensuring reconciliation happens for annotation changes")
			Eventually(func() int {
				return testMockController.GetEnsureCallCount()
			}, 3*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1))

			By("Verifying ServiceController methods were called")
			Expect(testMockController.GetEnsureCallCount()).To(BeNumerically(">=", 1))
			Expect(testMockController.GetDeleteCallCount()).To(Equal(0))
		})
	})
})
