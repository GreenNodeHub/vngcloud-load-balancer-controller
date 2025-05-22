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

	"github.com/anngdinh/operator-helper/contexts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/builder"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const (
	skipServiceTest = false
)

var _ = Describe("Service Controller", func() {
	Context("Wait 5 seconds before start test", func() {
		It("should be alright", func() {
			ctx = contexts.NewContext(ctx).SetLogName("___s___").GetContext()
			time.Sleep(5 * time.Second)
		})
	})

	Context("When create, update or delete a service", func() {
		It("should successfully reconcile the resource", func() {
			if skipServiceTest {
				Skip("Skip test")
			}
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
			if skipServiceTest {
				Skip("Skip test")
			}
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
			if skipServiceTest {
				Skip("Skip test")
			}
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
			if skipServiceTest {
				Skip("Skip test")
			}
			mockServiceReconciler.modeTest = false

			type stepType struct {
				name          string
				updateObjects func() []client.Object
				expect        func(lb *entity.LoadBalancer)
			}

			tests := []struct {
				name            string
				generateDepends func() []client.Object
				generateService func() *corev1.Service
				expect          func(lb *entity.LoadBalancer)
				steps           []stepType
			}{
				{
					name:            "create service with default annotation",
					generateDepends: func() []client.Object { return nil },
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-95466"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
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
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
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
					name:            "all normal annotations in the same time",
					generateDepends: func() []client.Object { return nil },
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
						Expect(loadbalancer.Internal).Should(BeTrue())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internal"))
						Expect(loadbalancer.PackageID).Should(Equal("package-iiiiiiiiiiiiiii"))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("SOURCE_IP"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("PING-UDP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(104))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(105))
							Expect(pool.HealthMonitor.Interval).Should(Equal(102))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(103))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
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
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("1.0.0.0/8"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
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
					name:            "create service with target node label, 1 label, 1 node",
					generateDepends: func() []client.Object { return nil },
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels): "nodeName=mock-node-1",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-95466"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(1)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
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
					name:            "create service with target node label, 2 label (AND logic), 1 node",
					generateDepends: func() []client.Object { return nil },
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels): "nodeName=mock-node-1,nodeGroup=mock-node-group-a",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-95466"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(1)) // number of member in pool = number of node OR number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								mockNode1.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
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
					name:            "service port use TCP protocol, but annotation use PROXY protocol",
					generateDepends: func() []client.Object { return nil },
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service-1", "default")
						service.Annotations = map[string]string{
							fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixEnableProxyProtocol): "*",
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-PRO-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("PROXY"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
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
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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
					name:            "update service with new port",
					generateDepends: func() []client.Object { return nil },
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service-1", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80)},
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(timeWaitRecocile)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
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
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
								Expect(loadbalancer.Internal).Should(BeFalse())
								Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
								Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
								Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 4"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

								// check pool
								pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(pools).ShouldNot(BeNil())
								Expect((pools.Items)).Should(HaveLen(1)) // number of pool
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-81"))
									Expect(pool.Description).Should(Equal("????????"))
									Expect(pool.Status).Should(Equal("ACTIVE"))
									Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
									Expect(pool.Protocol).Should(Equal("TCP"))
									Expect(pool.Stickiness).Should(BeFalse())
									Expect(pool.TLSEncryption).Should(BeFalse())

									Expect(pool.HealthMonitor).ShouldNot(BeNil())
									Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
									Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.Interval).Should(Equal(30))
									Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

									Expect(pool.Members).ShouldNot(BeNil())
									Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
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
								Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
								for _, listener := range listeners.Items {
									Expect(listener.Protocol).Should(Equal("TCP"))
									Expect(listener.ProtocolPort).Should(Equal(81))
									Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
									Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
									Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
									Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-81"))
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
				{
					name: "service target port is name, should find the port number in the endpoint when target type is ip",
					generateDepends: func() []client.Object {
						endpoint := newEndpointResource("test-service-1", "default")
						endpoint.Subsets = []corev1.EndpointSubset{
							// endpointSubset is for Deployment,... which is in use by service
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "100.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 8080},
									{Name: "https", Port: 6443},
								},
							},
						}
						return []client.Object{endpoint}
					},
					generateService: func() *corev1.Service {
						service := newServiceResource("test-service-1", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), NodePort: 31000},
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(timeWaitRecocile)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
						Expect(loadbalancer.Internal).Should(BeFalse())
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("TCP"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
							Expect(pool.HealthMonitor.Interval).Should(Equal(30))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
							for _, member := range pool.Members.Items {
								Expect(member.ProtocolPort).Should(Equal(31000))
								Expect(member.MonitorPort).Should(Equal(31000))
								Expect(member.Address).Should(BeElementOf(
									mockNode1.Status.Addresses[0].Address,
									mockNode2.Status.Addresses[0].Address,
									mockNode3.Status.Addresses[0].Address,
									mockNode4.Status.Addresses[0].Address))
							}
						}

						// check listener
						listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(listeners).ShouldNot(BeNil())
						Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("TCP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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
							name: "update service annotation with target type is ip",
							updateObjects: func() []client.Object {
								// get object
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-service-1", Namespace: "default"}, &object)).Should(Succeed())
								if object.Annotations == nil {
									object.Annotations = make(map[string]string)
								}
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetType)] = string(builder.TargetTypeIP)
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
								Expect(loadbalancer.Internal).Should(BeFalse())
								Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
								Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L4_PACKAGE_ID))
								Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 4"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

								// check pool
								pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(pools).ShouldNot(BeNil())
								Expect((pools.Items)).Should(HaveLen(1)) // number of pool
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
									Expect(pool.Description).Should(Equal("????????"))
									Expect(pool.Status).Should(Equal("ACTIVE"))
									Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
									Expect(pool.Protocol).Should(Equal("TCP"))
									Expect(pool.Stickiness).Should(BeFalse())
									Expect(pool.TLSEncryption).Should(BeFalse())

									Expect(pool.HealthMonitor).ShouldNot(BeNil())
									Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
									Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
									Expect(pool.HealthMonitor.Interval).Should(Equal(30))
									Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

									Expect(pool.Members).ShouldNot(BeNil())
									Expect((pool.Members.Items)).Should(HaveLen(8)) // number of member in pool = number of node or number of endpoint
									for _, member := range pool.Members.Items {
										Expect(member.ProtocolPort).Should(BeElementOf(80, 8080))
										Expect(member.MonitorPort).Should(BeElementOf(80, 8080))
										Expect(member.Address).Should(BeElementOf(
											"100.0.1.0", "100.0.2.0", "100.0.3.0", "100.0.4.0",
											"200.0.1.0", "200.0.2.0", "200.0.3.0", "200.0.4.0",
										))
									}
								}

								// check listener
								listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(listeners).ShouldNot(BeNil())
								Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
								for _, listener := range listeners.Items {
									Expect(listener.Protocol).Should(Equal("TCP"))
									Expect(listener.ProtocolPort).Should(Equal(80))
									Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
									Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
									Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
									Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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

				depends := tt.generateDepends()
				for _, depend := range depends {
					Expect(depend).NotTo(BeNil())
					Expect(k8sClient.Create(ctx, depend)).Should(Succeed())
				}

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
						logrus.Info("###### STEP: ", step.name)
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

	Context("When update tags and secgroups annotations", func() {
		It("load balancer and server should do expect behavior", func() {
			if skipServiceTest {
				Skip("Skip test")
			}
			mockServiceReconciler.modeTest = false

			// add 2 foo security group
			bigbangSec, err := mockProvider.CreateSecurityGroup(ctx, "bigbang", "the best security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(bigbangSec).ShouldNot(BeNil())
			blackpinkSec, err := mockProvider.CreateSecurityGroup(ctx, "blackpink", "the great security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blackpinkSec).ShouldNot(BeNil())

			// delete when finish
			defer func() {
				Expect(mockProvider.DeleteSecurityGroup(ctx, bigbangSec.Id)).Should(Succeed())
				Expect(mockProvider.DeleteSecurityGroup(ctx, blackpinkSec.Id)).Should(Succeed())
			}()

			type stepType struct {
				name          string                        // step name
				updateObjects func() []client.Object        // update objects such as ingress, service, endpoint,...
				expect        func(lb *entity.LoadBalancer) // expect after update
			}

			tests := []struct {
				preTest         func()                        // prepare test
				name            string                        // test name
				generateDepends func() []client.Object        // generate depend objects such as service, endpoint,...
				generateObj     func() client.Object          // generate main object
				expect          func(lb *entity.LoadBalancer) // expect after create
				steps           []stepType                    // update and expect for each step
				postTest        func()                        // expect after clean up
			}{
				{
					preTest: func() {
						mockServiceReconciler.cniMode = utils.CiliumNativeRouting
					},
					name: "create with default annotations of cilium native routing",
					generateDepends: func() []client.Object {
						endpoint := newEndpointResource("test-service-gogsf", "default")
						endpoint.Subsets = []corev1.EndpointSubset{
							// endpointSubset is for Deployment,... which is in use by service
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "100.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 8080},
									{Name: "https", Port: 6443},
								},
							},
						}
						return []client.Object{endpoint}
					},
					generateObj: func() client.Object {
						service := newServiceResource("test-service-gogsf", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), NodePort: 31000},
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(timeWaitRecocile)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

						// check tags
						tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(tags).ShouldNot(BeNil())
						Expect((tags.Items)).Should(HaveLen(1))
						Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))
						Expect(tags.Items[0].Value).Should(Equal(mockConfig.Cluster.ClusterID))

						// check secgroups
						secgroups, err := mockProvider.ListSecurityGroups(ctx)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(secgroups).ShouldNot(BeNil())
						Expect((secgroups.Items)).Should(HaveLen(3))
						expectName := []string{"vks-k8s-000000-default-test-servi-4d0e7", bigbangSec.Name, blackpinkSec.Name}
						secgroupID := ""
						for _, secgroup := range secgroups.Items {
							if secgroup.Name == "vks-k8s-000000-default-test-servi-4d0e7" {
								secgroupID = secgroup.Id
							}
							Expect(secgroup.Name).Should(BeElementOf(expectName))
							expectName = removeFisrt(expectName, secgroup.Name)
						}

						// check secgroup rule
						rules, err := mockProvider.ListSecurityGroupRules(ctx, secgroupID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(rules).ShouldNot(BeNil())
						Expect((rules.Items)).Should(HaveLen(3))
						expectPortRangeMax := []int{80, 8080, 31000} // cilium should only have nodeport + podport
						for _, rule := range rules.Items {
							Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
							expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
							Expect(rule.PortRangeMin).Should(Equal(rule.PortRangeMax))
							Expect(rule.Direction).Should(Equal("ingress"))
							Expect(rule.EtherType).Should(Equal("IPv4"))
							Expect(rule.Protocol).Should(Equal("tcp"))
							Expect(rule.RemoteIPPrefix).Should(Equal(provider.MockSubnetCIDR))
						}

						// check server have secgroup
						server, err := mockProvider.ListServerBySecgroupID(ctx, secgroupID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(server).ShouldNot(BeNil())
						Expect((server.Items)).Should(HaveLen(4))
						for _, item := range server.Items {
							serverSecgroups := make([]string, 0)
							for _, secgroup := range item.SecGroups {
								serverSecgroups = append(serverSecgroups, secgroup.Uuid)
							}
							Expect(serverSecgroups).Should(ContainElement(secgroupID))
						}
					},
					steps: []stepType{
						{
							name: "update endpoint, should update secgroup rule",
							updateObjects: func() []client.Object {
								object := corev1.Endpoints{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								object.Subsets = []corev1.EndpointSubset{
									// endpointSubset is for Deployment,... which is in use by service
									{
										Addresses: []corev1.EndpointAddress{
											{IP: "100.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
											{IP: "100.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
										},
										NotReadyAddresses: []corev1.EndpointAddress{
											{IP: "100.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
											{IP: "100.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
										},
										Ports: []corev1.EndpointPort{
											{Name: "http", Port: 80},
											{Name: "https", Port: 443},
										},
									},
								}
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

								// check tags
								tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(1))
								Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))
								Expect(tags.Items[0].Value).Should(Equal(mockConfig.Cluster.ClusterID))

								// check secgroups
								secgroups, err := mockProvider.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(3))
								expectName := []string{"vks-k8s-000000-default-test-servi-4d0e7", bigbangSec.Name, blackpinkSec.Name}
								secgroupID := ""
								for _, secgroup := range secgroups.Items {
									if secgroup.Name == "vks-k8s-000000-default-test-servi-4d0e7" {
										secgroupID = secgroup.Id
									}
									Expect(secgroup.Name).Should(BeElementOf(expectName))
									expectName = removeFisrt(expectName, secgroup.Name)
								}

								// check secgroup rule
								rules, err := mockProvider.ListSecurityGroupRules(ctx, secgroupID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(rules).ShouldNot(BeNil())
								Expect((rules.Items)).Should(HaveLen(2))
								expectPortRangeMax := []int{80, 31000} // cilium should only have nodeport + podport
								for _, rule := range rules.Items {
									Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
									expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
									Expect(rule.PortRangeMin).Should(Equal(rule.PortRangeMax))
									Expect(rule.Direction).Should(Equal("ingress"))
									Expect(rule.EtherType).Should(Equal("IPv4"))
									Expect(rule.Protocol).Should(Equal("tcp"))
									Expect(rule.RemoteIPPrefix).Should(Equal(provider.MockSubnetCIDR))
								}

								// check server have secgroup
								server, err := mockProvider.ListServerBySecgroupID(ctx, secgroupID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
								for _, item := range server.Items {
									serverSecgroups := make([]string, 0)
									for _, secgroup := range item.SecGroups {
										serverSecgroups = append(serverSecgroups, secgroup.Uuid)
									}
									Expect(serverSecgroups).Should(ContainElement(secgroupID))
								}
							},
						},
						{
							name: "update tags (add more tags) and secgroups annotations (delete default secgroup and add additional secgroups)",
							updateObjects: func() []client.Object {
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								if object.Annotations == nil {
									object.Annotations = map[string]string{}
								}
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag1=value1,tag2=value2"
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = bigbangSec.Id
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

								// check tags
								tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(3))
								expectKeys := []string{consts.VKS_TAG_KEY, "tag1", "tag2"}
								expectValues := []string{mockConfig.Cluster.ClusterID, "value1", "value2"}
								for _, tag := range tags.Items {
									Expect(tag.Key).Should(BeElementOf(expectKeys))
									expectKeys = removeFisrt(expectKeys, tag.Key)
									Expect(tag.Value).Should(BeElementOf(expectValues))
									expectValues = removeFisrt(expectValues, tag.Value)
								}

								// check secgroups
								secgroups, err := mockProvider.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should delete default secgroup

								// check server have secgroup
								server, err := mockProvider.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
						{
							name: "update tags (remove, update, add tags) and secgroups annotations (remove and add secgroups in server)",
							updateObjects: func() []client.Object {
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								if object.Annotations == nil {
									object.Annotations = map[string]string{}
								}
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag2=value22, tag3=value3"
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = blackpinkSec.Id
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

								// check tags
								tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(3))
								expectKeys := []string{consts.VKS_TAG_KEY, "tag2", "tag3"}
								expectValues := []string{mockConfig.Cluster.ClusterID, "value22", "value3"}
								for _, tag := range tags.Items {
									Expect(tag.Key).Should(BeElementOf(expectKeys))
									expectKeys = removeFisrt(expectKeys, tag.Key)
									Expect(tag.Value).Should(BeElementOf(expectValues))
									expectValues = removeFisrt(expectValues, tag.Value)
								}

								// check secgroups
								secgroups, err := mockProvider.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should not delete secgroup

								// check server have secgroup
								server, err := mockProvider.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(BeEmpty())

								server, err = mockProvider.ListServerBySecgroupID(ctx, blackpinkSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
					},
					postTest: func() {},
				},
				{
					preTest: func() {
						mockServiceReconciler.cniMode = utils.CalicoOverlay
					},
					name: "create with default annotations of calico overlay",
					generateDepends: func() []client.Object {
						endpoint := newEndpointResource("test-service-gogsf", "default")
						endpoint.Subsets = []corev1.EndpointSubset{
							// endpointSubset is for Deployment,... which is in use by service
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "100.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &mockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &mockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &mockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &mockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 8080},
									{Name: "https", Port: 6443},
								},
							},
						}

						return []client.Object{endpoint}
					},
					generateObj: func() client.Object {
						service := newServiceResource("test-service-gogsf", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
						}
						return service
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(2 * timeWaitRecocile)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

						// check tags
						tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(tags).ShouldNot(BeNil())
						Expect((tags.Items)).Should(HaveLen(1))
						Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))
						Expect(tags.Items[0].Value).Should(Equal(mockConfig.Cluster.ClusterID))

						// check secgroups
						secgroups, err := mockProvider.ListSecurityGroups(ctx)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(secgroups).ShouldNot(BeNil())
						Expect((secgroups.Items)).Should(HaveLen(3))
						expectName := []string{"vks-k8s-000000-default-test-servi-4d0e7", bigbangSec.Name, blackpinkSec.Name}
						secgroupID := ""
						for _, secgroup := range secgroups.Items {
							if secgroup.Name == "vks-k8s-000000-default-test-servi-4d0e7" {
								secgroupID = secgroup.Id
							}
							Expect(secgroup.Name).Should(BeElementOf(expectName))
							expectName = removeFisrt(expectName, secgroup.Name)
						}

						// check secgroup rule
						rules, err := mockProvider.ListSecurityGroupRules(ctx, secgroupID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(rules).ShouldNot(BeNil())
						Expect((rules.Items)).Should(HaveLen(1))
						expectPortRangeMax := []int{30000} // calico overlay should only have nodeport
						for _, rule := range rules.Items {
							Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
							expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
							Expect(rule.PortRangeMin).Should(Equal(rule.PortRangeMax))
							Expect(rule.Direction).Should(Equal("ingress"))
							Expect(rule.EtherType).Should(Equal("IPv4"))
							Expect(rule.Protocol).Should(Equal("tcp"))
							Expect(rule.RemoteIPPrefix).Should(Equal(provider.MockSubnetCIDR))
						}

						// check server have secgroup
						server, err := mockProvider.ListServerBySecgroupID(ctx, secgroupID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(server).ShouldNot(BeNil())
						Expect((server.Items)).Should(HaveLen(4))
						for _, item := range server.Items {
							serverSecgroups := make([]string, 0)
							for _, secgroup := range item.SecGroups {
								serverSecgroups = append(serverSecgroups, secgroup.Uuid)
							}
							Expect(serverSecgroups).Should(ContainElement(secgroupID))
						}
					},
					steps: []stepType{
						{
							name: "update tags (add more tags) and secgroups annotations (delete default secgroup and add additional secgroups)",
							updateObjects: func() []client.Object {
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								if object.Annotations == nil {
									object.Annotations = map[string]string{}
								}
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag1=value1,tag2=value2"
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = bigbangSec.Id
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

								// check tags
								tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(3))
								expectKeys := []string{consts.VKS_TAG_KEY, "tag1", "tag2"}
								expectValues := []string{mockConfig.Cluster.ClusterID, "value1", "value2"}
								for _, tag := range tags.Items {
									Expect(tag.Key).Should(BeElementOf(expectKeys))
									expectKeys = removeFisrt(expectKeys, tag.Key)
									Expect(tag.Value).Should(BeElementOf(expectValues))
									expectValues = removeFisrt(expectValues, tag.Value)
								}

								// check secgroups
								secgroups, err := mockProvider.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should delete default secgroup

								// check server have secgroup
								server, err := mockProvider.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
						{
							name: "update tags (remove, update, add tags) and secgroups annotations (remove and add secgroups in server)",
							updateObjects: func() []client.Object {
								object := corev1.Service{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								if object.Annotations == nil {
									object.Annotations = map[string]string{}
								}
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag2=value22, tag3=value3"
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = blackpinkSec.Id
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-4d0e7"))

								// check tags
								tags, err := mockProvider.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(3))
								expectKeys := []string{consts.VKS_TAG_KEY, "tag2", "tag3"}
								expectValues := []string{mockConfig.Cluster.ClusterID, "value22", "value3"}
								for _, tag := range tags.Items {
									Expect(tag.Key).Should(BeElementOf(expectKeys))
									expectKeys = removeFisrt(expectKeys, tag.Key)
									Expect(tag.Value).Should(BeElementOf(expectValues))
									expectValues = removeFisrt(expectValues, tag.Value)
								}

								// check secgroups
								secgroups, err := mockProvider.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should not delete secgroup

								// check server have secgroup
								server, err := mockProvider.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(BeEmpty())

								server, err = mockProvider.ListServerBySecgroupID(ctx, blackpinkSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
					},
					postTest: func() {},
				},
			}

			for _, tt := range tests {
				logrus.Info("------------------- ", tt.name, " -------------------")
				time.Sleep(timeWaitRecocile)
				if tt.preTest != nil {
					tt.preTest()
				}
				depends := tt.generateDepends()
				for _, depend := range depends {
					Expect(depend).NotTo(BeNil())
					Expect(k8sClient.Create(ctx, depend)).Should(Succeed())
				}

				obj := tt.generateObj()
				Expect(obj).NotTo(BeNil())
				Expect(k8sClient.Create(ctx, obj)).Should(Succeed())

				// get load balancer id in the annotation
				loadbalancerID := ""
				Eventually(func() bool {
					getObj := &corev1.Service{}
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, getObj)).Should(Succeed())
					loadbalancerID = getObj.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
					return loadbalancerID != ""
				}, timeout, interval).Should(BeTrue())

				// expect load balancer attribute in the mock provider
				loadbalancer, err := mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
				Expect(err).ShouldNot(HaveOccurred())
				tt.expect(loadbalancer)

				if tt.steps != nil {
					for _, step := range tt.steps {
						logrus.Info("###### STEP: ", step.name)
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
				Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
				Eventually(func() bool {
					getObj := &corev1.Service{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, getObj)
					return err != nil
				}, 2*timeout, interval).Should(BeTrue())
				_, err = mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
				Expect(err).Should(HaveOccurred())

				for _, depend := range depends {
					// delete depend and ignore not found error
					err := k8sClient.Delete(ctx, depend)
					Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
					err = k8sClient.Get(ctx, client.ObjectKey{Name: depend.GetName(), Namespace: depend.GetNamespace()}, depend)
					Expect(err).Should(HaveOccurred())
				}
				if tt.postTest != nil {
					tt.postTest()
				}
				printEndTest()
			}
		})
	})

	Context("Test create LB error", func() {
		It("it should work as expectation", func() {
			if skipServiceTest {
				Skip("Skip test")
			}
			mockServiceReconciler.modeTest = false

			test := TestType[*corev1.Service]{
				preTest:         func() {},
				postTest:        func() {},
				name:            "create service with name will be error",
				generateDepends: func() []client.Object { return []client.Object{} },
				generateObj: func() []ObjectAndExpect[*corev1.Service] {
					service := newServiceResource("test-service-error", "default")
					service.Spec.Ports = []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
					}
					if service.Annotations == nil {
						service.Annotations = map[string]string{}
					}
					service.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerName)] = provider.MockLBNameError
					return []ObjectAndExpect[*corev1.Service]{{obj: service, expect: func() {}}}
				},
				expect: func() {
					// wait until reconcile done
					time.Sleep(timeWaitRecocile)

					// it will create and delete load balancer continuously because of error
					Eventually(func() int {
						listLB, err := mockProvider.ListLoadBalancers(ctx, nil)
						Expect(err).ShouldNot(HaveOccurred())
						return len(listLB.Items)
					}, timeout, interval).Should(Equal(0))
				},
				steps: []StepType{
					{
						kindStep: updateStep,
						name:     "update lb name to normal",
						getObject: func() client.Object {
							obj := &corev1.Service{}
							Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-error", Namespace: "default"}, obj)).Should(Succeed())
							obj.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerName)] = "normal-name"
							return obj
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(timeWaitRecocile)

							Eventually(func() int {
								listLB, err := mockProvider.ListLoadBalancers(ctx, nil)
								Expect(err).ShouldNot(HaveOccurred())
								return len(listLB.Items)
							}, timeout, interval).Should(Equal(1))

							// get load balancer by id in resource annotation
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-error", Namespace: "default"}}
							loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
							Expect(loadbalancer).ShouldNot(BeNil())
							Expect(loadbalancer.Name).Should(Equal("normal-name"))
						},
					},
				},
				expectAfterDelete: func() {},
			}

			logrus.Info("Running test: ", test.name)
			RunMultiStepTest[*corev1.Service](test)
		})
	})

	Context("When update target node label", func() {
		It("it should update default secgroup in server opt out", func() {
			if skipIngressTest {
				Skip("Skip test")
			}
			mockServiceReconciler.modeTest = false

			test := TestType[*corev1.Service]{
				preTest:         func() {},
				postTest:        func() {},
				name:            "create service with target node label",
				generateDepends: func() []client.Object { return []client.Object{} },
				generateObj: func() []ObjectAndExpect[*corev1.Service] {
					service := newServiceResource("test-service-target-node-label", "default")
					service.Spec.Ports = []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
					}
					if service.Annotations == nil {
						service.Annotations = map[string]string{}
					}
					service.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels)] = "nodeGroup=mock-node-group-a"
					return []ObjectAndExpect[*corev1.Service]{{obj: service, expect: func() {}}}
				},
				expect: func() {
					// wait until reconcile done
					time.Sleep(timeWaitRecocile)

					// get load balancer by id in resource annotation
					obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-target-node-label", Namespace: "default"}}
					loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
					Expect(loadbalancer).ShouldNot(BeNil())

					// check pool
					pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pools).ShouldNot(BeNil())
					Expect((pools.Items)).Should(HaveLen(1)) // number of pool
					for _, pool := range pools.Items {
						Expect(pool.Members).ShouldNot(BeNil())
						Expect((pool.Members.Items)).Should(HaveLen(2)) // number of member in pool = number of node or number of endpoint
						expectAddress := []string{
							mockNode1.Status.Addresses[0].Address,
							mockNode2.Status.Addresses[0].Address}
						for _, member := range pool.Members.Items {
							Expect(member.MonitorPort).Should(Equal(member.ProtocolPort))
							Expect(member.Address).Should(BeElementOf(expectAddress))
							expectAddress = removeFisrt(expectAddress, member.Address)
						}
					}

					// check secgroups
					secgroups, err := mockProvider.ListSecurityGroups(ctx)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(secgroups).ShouldNot(BeNil())
					Expect((secgroups.Items)).Should(HaveLen(1))
					secgroupID := ""
					for _, secgroup := range secgroups.Items {
						if secgroup.Name == loadbalancer.Name {
							secgroupID = secgroup.Id
						}
					}
					Expect(secgroupID).ShouldNot(BeEmpty())

					// check server have secgroup
					server, err := mockProvider.ListServerBySecgroupID(ctx, secgroupID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(server).ShouldNot(BeNil())
					Expect((server.Items)).Should(HaveLen(2))
					expectServerID := []string{"ins-00000000-0000-0000-0000-000000000001", "ins-00000000-0000-0000-0000-000000000002"}
					for _, item := range server.Items {
						Expect(item.Uuid).Should(BeElementOf(expectServerID))
						expectServerID = removeFisrt(expectServerID, item.Name)

						serverSecgroups := make([]string, 0)
						for _, secgroup := range item.SecGroups {
							serverSecgroups = append(serverSecgroups, secgroup.Uuid)
						}
						Expect(serverSecgroups).Should(ContainElement(secgroupID))
					}

					// check server opt out
					unexpectedServerID := []string{"ins-00000000-0000-0000-0000-000000000003", "ins-00000000-0000-0000-0000-000000000004"}
					for _, item := range server.Items {
						Expect(item.Uuid).ShouldNot(BeElementOf(unexpectedServerID))
					}
				},
				steps: []StepType{
					{
						kindStep: updateStep,
						name:     "update target node label",
						getObject: func() client.Object {
							obj := &corev1.Service{}
							Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-target-node-label", Namespace: "default"}, obj)).Should(Succeed())
							obj.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels)] = "nodeGroup=mock-node-group-b"
							return obj
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-target-node-label", Namespace: "default"}}
							loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(1)) // number of pool
							for _, pool := range pools.Items {
								Expect(pool.Members).ShouldNot(BeNil())
								Expect((pool.Members.Items)).Should(HaveLen(2)) // number of member in pool = number of node or number of endpoint
								expectAddress := []string{
									mockNode3.Status.Addresses[0].Address,
									mockNode4.Status.Addresses[0].Address}
								for _, member := range pool.Members.Items {
									Expect(member.MonitorPort).Should(Equal(member.ProtocolPort))
									Expect(member.Address).Should(BeElementOf(expectAddress))
									expectAddress = removeFisrt(expectAddress, member.Address)
								}
							}

							// check secgroups
							secgroups, err := mockProvider.ListSecurityGroups(ctx)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(secgroups).ShouldNot(BeNil())
							Expect((secgroups.Items)).Should(HaveLen(1))
							secgroupID := ""
							for _, secgroup := range secgroups.Items {
								if secgroup.Name == loadbalancer.Name {
									secgroupID = secgroup.Id
								}
							}
							Expect(secgroupID).ShouldNot(BeEmpty())

							// check server have secgroup
							server, err := mockProvider.ListServerBySecgroupID(ctx, secgroupID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(server).ShouldNot(BeNil())
							Expect((server.Items)).Should(HaveLen(2))
							expectServerID := []string{"ins-00000000-0000-0000-0000-000000000003", "ins-00000000-0000-0000-0000-000000000004"}
							for _, item := range server.Items {
								Expect(item.Uuid).Should(BeElementOf(expectServerID))
								expectServerID = removeFisrt(expectServerID, item.Name)

								serverSecgroups := make([]string, 0)
								for _, secgroup := range item.SecGroups {
									serverSecgroups = append(serverSecgroups, secgroup.Uuid)
								}
								Expect(serverSecgroups).Should(ContainElement(secgroupID))
							}

							// check server opt out
							unexpectedServerID := []string{"ins-00000000-0000-0000-0000-000000000001", "ins-00000000-0000-0000-0000-000000000002"}
							for _, item := range server.Items {
								Expect(item.Uuid).ShouldNot(BeElementOf(unexpectedServerID))
							}
						},
					},
				},
				expectAfterDelete: func() {},
			}

			logrus.Info("Running test: ", test.name)
			RunMultiStepTest[*corev1.Service](test)
		})
	})

	Context("When create 3 service using same LB", func() {
		It("it should work well, delete should delete all", func() {
			if skipIngressTest {
				Skip("Skip test")
			}
			mockServiceReconciler.modeTest = false

			lbID := ""
			test := TestType[*corev1.Service]{
				preTest:         func() {},
				postTest:        func() {},
				name:            "create service normal",
				generateDepends: func() []client.Object { return []client.Object{} },
				generateObj: func() []ObjectAndExpect[*corev1.Service] {
					service := newServiceResource("test-service-port-80", "default")
					service.Spec.Ports = []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
					}
					return []ObjectAndExpect[*corev1.Service]{{obj: service, expect: func() {}}}
				},
				expect: func() {
					// wait until reconcile done
					time.Sleep(timeWaitRecocile)

					// get load balancer by id in resource annotation
					obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-80", Namespace: "default"}}
					loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
					lbID = loadbalancer.UUID
					Expect(loadbalancer).ShouldNot(BeNil())

					// check pool
					pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pools).ShouldNot(BeNil())
					Expect((pools.Items)).Should(HaveLen(1)) // number of pool

					// check listener
					listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(listeners).ShouldNot(BeNil())
					Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
				},
				steps: []StepType{
					{
						kindStep: createStep,
						name:     "create new service with same LB ID annotation",
						getObject: func() client.Object {
							service := newServiceResource("test-service-port-81", "default")
							service.Spec.Ports = []corev1.ServicePort{
								{Name: "http", Port: 81, TargetPort: intstr.FromInt(81), Protocol: corev1.ProtocolTCP, NodePort: 30001},
							}
							if service.Annotations == nil {
								service.Annotations = map[string]string{}
							}
							service.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)] = lbID
							return service
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-80", Namespace: "default"}}
							loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
							lbID = loadbalancer.UUID
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(2)) // number of pool

							// check listener
							listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(listeners).ShouldNot(BeNil())
							Expect((listeners.Items)).Should(HaveLen(2)) // number of listener
						},
					},
					{
						kindStep: createStep,
						name:     "create new service with same LB ID annotation",
						getObject: func() client.Object {
							service := newServiceResource("test-service-port-82", "default")
							service.Spec.Ports = []corev1.ServicePort{
								{Name: "http", Port: 82, TargetPort: intstr.FromInt(82), Protocol: corev1.ProtocolTCP, NodePort: 30002},
							}
							if service.Annotations == nil {
								service.Annotations = map[string]string{}
							}
							service.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)] = lbID
							return service
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-80", Namespace: "default"}}
							loadbalancer := getLBByAnnotation[*corev1.Service](k8sClient, obj)
							lbID = loadbalancer.UUID
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(3)) // number of pool

							// check listener
							listeners, err := mockProvider.ListListenerOfLB(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(listeners).ShouldNot(BeNil())
							Expect((listeners.Items)).Should(HaveLen(3)) // number of listener
						},
					},
					{
						kindStep: deleteStep,
						name:     "delete service",
						getObject: func() client.Object {
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-80", Namespace: "default"}}
							return obj
						},
						expect: func() {},
					},
					{
						kindStep: deleteStep,
						name:     "delete service",
						getObject: func() client.Object {
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-81", Namespace: "default"}}
							return obj
						},
						expect: func() {},
					},
					{
						kindStep: deleteStep,
						name:     "delete service",
						getObject: func() client.Object {
							obj := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service-port-82", Namespace: "default"}}
							return obj
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(timeWaitRecocile)

							// it will delete load balancer
							Eventually(func() int {
								listLB, err := mockProvider.ListLoadBalancers(ctx, nil)
								Expect(err).ShouldNot(HaveOccurred())
								return len(listLB.Items)
							}, timeout, interval).Should(Equal(0))
						},
					},
				},
				expectAfterDelete: func() {},
			}

			logrus.Info("Running test: ", test.name)
			RunMultiStepTest[*corev1.Service](test)
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

var _ = newServiceResource("placeholder", "placeholder")

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
