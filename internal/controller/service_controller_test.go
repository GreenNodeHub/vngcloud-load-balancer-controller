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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/contexts"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	Context("Wait 5 seconds before start test", func() {
		It("should be alright", func() {
			ctx = contexts.NewContext(ctx).SetLogName("___s___").GetContext()
			time.Sleep(5 * time.Second)
		})
	})

	Context("When create, update or delete a service", func() {
		It("should successfully reconcile the resource", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Service: ", req, ", countReconcile: ", countReconcile)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.ensureTest = funcTest
			mockServiceReconciler.deleteTest = funcTest

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newServiceResource("test-service", "default")
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
			service = newServiceResource("test-service-2", "default")
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
				klog.Info("Reconcile Service: ", req, ", countReconcile: ", countReconcile)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.ensureTest = funcTest
			mockServiceReconciler.deleteTest = funcTest

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newServiceResource("test-service", "default")
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
				klog.Info("Reconcile Service: ", req, ", countReconcile: ", countReconcile)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockServiceReconciler.deleteTest = func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcileDelete++
				klog.Info("Delete Service: ", req, ", countReconcileDelete: ", countReconcileDelete)
				time.Sleep(1 * time.Second)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			klog.Info("countReconcile: ", countReconcile, " countReconcileDelete: ", countReconcileDelete)

			// when create a service LoadBalancer type, the controller will reconcile it
			service := newServiceResource("test-service", "default")
			Expect(service).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				klog.Info("countReconcile: ", countReconcile, " countReconcileDelete: ", countReconcileDelete)
				return countReconcile == 1 && countReconcileDelete == 0
			}, timeout, interval).Should(BeTrue())

			// when update a service LoadBalancer type to the other type, the controller will reconcile it as a delete event
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeNodePort
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1 && countReconcileDelete == 1
			}, timeout, interval).Should(BeTrue())

			// when update a service not LoadBalancer type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Annotations = map[string]string{"test": "test"}
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1 && countReconcileDelete == 1
			}, timeout, interval).Should(BeTrue())

			// when update a service to LoadBalancer type, the controller will reconcile it as a create event
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			service.Spec.Type = corev1.ServiceTypeLoadBalancer
			Expect(k8sClient.Update(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 2 && countReconcileDelete == 1
			}, timeout, interval).Should(BeTrue())

			// when delete a service LoadBalancer type, the controller will not reconcile it
			service = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service", Namespace: "default"}, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 2 && countReconcileDelete == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When create service with specific annotation", func() {
		It("created load balancer shoud have specific attribute", func() {
			mockServiceReconciler.modeTest = false

			type stepType struct {
				name          string
				updateObjects func() []client.Object
				expect        func(lb *entity.LoadBalancer)
			}

			tests := []struct {
				name            string
				generateService func() *corev1.Service
				expect          func(lb *entity.LoadBalancer)
				steps           []stepType
			}{
				{
					name: "create service with default annotation",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-fbaa0"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address,
								mockNode2.Status.Addresses[0].Address,
								mockNode3.Status.Addresses[0].Address,
								mockNode4.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(50))
							Expect(listener.TimeoutConnection).Should(Equal(5))
							Expect(listener.TimeoutMember).Should(Equal(50))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: nil,
				},
				{
					name: "all normal annotations in the same time",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerName):           "test-lb",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixPackageID):                  "package-iiiiiiiiiiiiiii",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixScheme):                     "internal",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixIdleTimeoutClient):          "99",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixIdleTimeoutMember):          "100",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixIdleTimeoutConnection):      "101",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixInboundCIDRs):               "1.0.0.0/8",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixHealthcheckPort):            "8888",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixHealthcheckProtocol):        "PING-UDP",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixHealthcheckIntervalSeconds): "102",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixHealthcheckTimeoutSeconds):  "103",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixHealthyThresholdCount):      "104",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixUnhealthyThresholdCount):    "105",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixPoolAlgorithm):              "SOURCE_IP",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixEnableAutoscale):            "true",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags):                       "tag1=656,tag2=5324",
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups):             "sg-1,sg-2",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("test-lb"))
						Expect(loadbalancer.Internal).Should(Equal(true))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internal"))
						Expect(loadbalancer.PackageID).Should(Equal("package-iiiiiiiiiiiiiii"))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("SOURCE_IP"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("PING-UDP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(104))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(105))
							Expect(pool.HealthMonitor.Interval).Should(Equal(102))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(103))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address,
								mockNode2.Status.Addresses[0].Address,
								mockNode3.Status.Addresses[0].Address,
								mockNode4.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("1.0.0.0/8"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(99))
							Expect(listener.TimeoutConnection).Should(Equal(101))
							Expect(listener.TimeoutMember).Should(Equal(100))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: nil,
				},
				{
					name: "create service with target node label, 1 label, 1 node",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels): "nodeName=mock-node-1",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-fbaa0"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(1)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(50))
							Expect(listener.TimeoutConnection).Should(Equal(5))
							Expect(listener.TimeoutMember).Should(Equal(50))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: nil,
				},
				{
					name: "create service with target node label, 2 label (AND logic), 1 node",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels): "nodeName=mock-node-1,nodeGroup=mock-node-group-a",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-fbaa0"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(1)) // number of member in pool = number of node OR number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(50))
							Expect(listener.TimeoutConnection).Should(Equal(5))
							Expect(listener.TimeoutMember).Should(Equal(50))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: nil,
				},
				{
					name: "service port use TCP protocol, but annotation use PROXY protocol",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service-1", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixEnableProxyProtocol): "*",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-e3551"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-PRO-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("PROXY"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address,
								mockNode2.Status.Addresses[0].Address,
								mockNode3.Status.Addresses[0].Address,
								mockNode4.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(50))
							Expect(listener.TimeoutConnection).Should(Equal(5))
							Expect(listener.TimeoutMember).Should(Equal(50))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: nil,
				},
				{
					name: "update service with new port",
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service-1", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80)},
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(20 * time.Second)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-e3551"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(Equal(false))
							Expect(pool.TLSEncryption).Should(Equal(false))

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect(len(pool.Members.Items)).Should(Equal(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address,
								mockNode2.Status.Addresses[0].Address,
								mockNode3.Status.Addresses[0].Address,
								mockNode4.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-TCP-80"))
							Expect(listener.TimeoutClient).Should(Equal(50))
							Expect(listener.TimeoutConnection).Should(Equal(5))
							Expect(listener.TimeoutMember).Should(Equal(50))
							Expect(listener.Description).Should(Equal("????????"))
							// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
							// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
						}
					},
					steps: []stepType{
						{
							name: "update service with new port, 80->81, should delete the old listener and pool",
							updateObjects: func() []client.Object {
								// get object
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-service-1", Namespace: "default"}, &object)).Should(Succeed())
								object.Spec.Ports = []corev1.ServicePort{
									{Name: "http", Port: 81, TargetPort: intstr.FromInt(80)},
								}
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(20 * time.Second)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-e3551"))
								Expect(loadbalancer.Internal).Should(Equal(false))
								Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
								Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L4_PACKAGE_ID))
								Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 4"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

								// check pool
								pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(pools).ShouldNot(BeNil())
								Expect(len(pools.Items)).Should(Equal(1)) // number of pool
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-TCP-81"))
									Expect(pool.Description).Should(Equal("????????"))
									Expect(pool.Status).Should(Equal("ACTIVE"))
									Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
									Expect(pool.Protocol).Should(Equal("TCP"))
									Expect(pool.Stickiness).Should(Equal(false))
									Expect(pool.TLSEncryption).Should(Equal(false))

									Expect(pool.HealthMonitor).ShouldNot(BeNil())
									Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
									Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.Interval).Should(Equal(30))
									Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

									Expect(pool.Members).ShouldNot(BeNil())
									Expect(len(pool.Members.Items)).Should(Equal(4)) // number of member in pool = number of node or number of endpoint
									Expect(pool.Members.Items[0].Address).Should(BeElementOf(
										mockNode1.Status.Addresses[0].Address,
										mockNode2.Status.Addresses[0].Address,
										mockNode3.Status.Addresses[0].Address,
										mockNode4.Status.Addresses[0].Address))
								}

								// check listener
								listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(listeners).ShouldNot(BeNil())
								Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
								for _, listener := range listeners.Items {
									Expect(listener.Protocol).Should(Equal("TCP"))
									Expect(listener.ProtocolPort).Should(Equal(81))
									Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
									Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
									Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
									Expect(listener.Name).Should(Equal("vks-test-clust-default-test-serv-e3551-TCP-81"))
									Expect(listener.TimeoutClient).Should(Equal(50))
									Expect(listener.TimeoutConnection).Should(Equal(5))
									Expect(listener.TimeoutMember).Should(Equal(50))
									Expect(listener.Description).Should(Equal("????????"))
									// Expect(listener.Headers).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.DisplayStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.ProgressStatus).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.UpdatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.CertificateAuthorities).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.ClientCertificateAuthentication).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.ConnectionLimit).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.CreatedAt).Should(Equal(aaaaaaaaaaaaaaaaaaa))
									// Expect(listener.DefaultCertificateAuthority).Should(Equal(aaaaaaaaaaaaaaaaaaa))
								}
							},
						},
					},
				},
				// {
				// 	name: "service port use UDP protocol",
				// },
				// {
				// 	name: "http/https healthcheck protocol",
				// },
				// {
				// 	name: "when user update service with new port (8080->8081), the controller should delete the old listener",
				// },
				// {
				// 	name: "load balancer is updating when action create pool, should wait until the load balancer is ready",
				// },
				// {
				// 	name: "____________________",
				// },
				// {
				// 	name: "____________________",
				// },
				// {
				// 	name: "____________________",
				// },
				// {
				// 	name: "____________________",
				// },
				// {
				// 	name: "____________________",
				// },
			}

			for _, tt := range tests {
				logrus.Info("------------------- ", tt.name, " -------------------")
				service := tt.generateService()
				Expect(service).NotTo(BeNil())
				Expect(k8sClient.Create(ctx, service)).Should(Succeed())

				// get load balancer id in the service annotation
				loadbalancerID := ""
				Eventually(func() bool {
					getService := &corev1.Service{}
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, getService)).Should(Succeed())
					loadbalancerID = getService.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
					return loadbalancerID != ""
				}, timeout, interval).Should(BeTrue())

				// expect load balancer attribute in the mock provider
				loadbalancer, err := mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
				Expect(err).ShouldNot(HaveOccurred())
				tt.expect(loadbalancer)

				if tt.steps != nil {
					for _, step := range tt.steps {
						logrus.Info("STEP: ", step.name)
						updateObjs := step.updateObjects()
						for _, obj := range updateObjs {
							Expect(obj).NotTo(BeNil())
							Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
						}

						// expect load balancer attribute in the mock provider
						step.expect(loadbalancer)
					}
				}

				// clean up
				Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
				Eventually(func() bool {
					getService := &corev1.Service{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, getService)
					return err != nil
				}, 2*timeout, interval).Should(BeTrue())
				_, err = mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
				Expect(err).Should(HaveOccurred())
				printEndTest()
			}
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

func newServiceResource(name, namespace string) *corev1.Service {
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
