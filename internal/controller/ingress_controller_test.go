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

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Ingress Controller", func() {
	Context("Wait 5 seconds before start test", func() {
		It("should be alright", func() {
			ctx = contexts.NewContext(ctx).SetLogName("___i___").GetContext()
			time.Sleep(5 * time.Second)
		})
	})

	Context("When update status", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest
			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			// update status
			ingress = &networkingv1.Ingress{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())

			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "10.0.0.1"}}
			Expect(k8sClient.Status().Update(ctx, ingress)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())
				return ingress != nil &&
					len(ingress.Status.LoadBalancer.Ingress) > 0 &&
					ingress.Status.LoadBalancer.Ingress[0].IP == "10.0.0.1"
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update status inside reconcile", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)

				// update status
				ingress := &networkingv1.Ingress{}
				k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)

				ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{IP: "10.0.0.1"}}
				k8sClient.Status().Update(ctx, ingress)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)).Should(Succeed())
				return ingress != nil &&
					len(ingress.Status.LoadBalancer.Ingress) > 0 &&
					ingress.Status.LoadBalancer.Ingress[0].IP == "10.0.0.1"
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update annotaion", func() {
		It("should reconcile immediately", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			updateIngressAnnotation("test-ingress", "default", "test", "test")
			Eventually(func() bool {
				return countReconcile == 2
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 3
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When update annotation in whitelist annotation", func() {
		It("should not reconcile", func() {
			countReconcile := 0
			funcTest := func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
				countReconcile++
				klog.Info("Reconcile Ingress: ", req)
				klog.Info("Done: ", req)
				return ctrl.Result{}, nil
			}
			mockIngressReconciler.ensureTest = funcTest
			mockIngressReconciler.deleteTest = funcTest

			ingress := newIngressResource("test-ingress", "default")
			Expect(ingress).NotTo(BeNil())
			Expect(ingress.Name).To(Equal("test-ingress"))
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				return countReconcile == 1
			}, timeout, interval).Should(BeTrue())

			updateIngressAnnotation("test-ingress", "default", "example.com/whitelist-annotation-1", "test")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(1))

			updateIngressAnnotation("test-ingress", "default", "example.com/blacklist-annotation-1", "test")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			updateIngressAnnotation("test-ingress", "default", "example.com/whitelist-annotation-1", "-")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(2))

			updateIngressAnnotation("test-ingress", "default", "example.com/blacklist-annotation-1", "-")
			Eventually(func() int {
				return countReconcile
			}, timeout, interval).Should(Equal(3))

			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-ingress", Namespace: "default"}, ingress)
				return err != nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				return countReconcile == 4
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When create ingress with specific annotation", func() {
		It("created load balancer shoud have specific attribute", func() {
			mockIngressReconciler.modeTest = false

			type stepType struct {
				name          string
				updateObjects func() []client.Object
				expect        func(lb *entity.LoadBalancer)
			}

			tests := []struct {
				name            string
				generateDepends func() []client.Object
				generateObj     func() client.Object
				expect          func(lb *entity.LoadBalancer)
				steps           []stepType
			}{
				{
					name: "create with default annotation",
					generateDepends: func() []client.Object {
						service := newServiceNodePortResource("test-service-gogsf", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
						}
						return []client.Object{service}
					},
					generateObj: func() client.Object {
						ingress := newIngressResource("test-service-gogsf", "default")
						Expect(ingress).NotTo(BeNil())
						ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: "test-service-gogsf",
								Port: networkingv1.ServiceBackendPort{
									Number: 80,
								},
							},
						}
						return ingress
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-9ec42"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L7_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 7"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(BeElementOf(
								consts.DEFAULT_NAME_DEFAULT_POOL,
								"vks-test-clust-default-test-serv-fbaa0-TCP-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("HTTP"))
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
							for _, member := range pool.Members.Items {
								Expect(member.ProtocolPort).Should(Equal(30000))
								Expect(member.MonitorPort).Should(Equal(30000))
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
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("HTTP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
							Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
							Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
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
				{
					name: "create without default backend, only 1 rule",
					generateDepends: func() []client.Object {
						service := newServiceNodePortResource("test-service-gogsf", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
							{Name: "https", Port: 443, TargetPort: intstr.FromInt(81), Protocol: corev1.ProtocolTCP, NodePort: 30001},
						}
						return []client.Object{service}
					},
					generateObj: func() client.Object {
						ingress := newIngressResource("test-service-gogsf", "default")
						Expect(ingress).NotTo(BeNil())
						ingress.Spec.DefaultBackend = nil
						ingress.Spec.Rules = []networkingv1.IngressRule{
							{
								Host: "test.com",
								IngressRuleValue: networkingv1.IngressRuleValue{
									HTTP: &networkingv1.HTTPIngressRuleValue{
										Paths: []networkingv1.HTTPIngressPath{
											{
												PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
												Path:     "/",
												Backend: networkingv1.IngressBackend{
													Service: &networkingv1.IngressServiceBackend{
														Name: "test-service-gogsf",
														Port: networkingv1.ServiceBackendPort{Number: 80},
													},
												},
											},
										},
									},
								},
							},
						}
						return ingress
					},
					expect: func(loadbalancer *entity.LoadBalancer) {
						// wait until reconcile done
						time.Sleep(20 * time.Second)

						Expect(loadbalancer).ShouldNot(BeNil())
						Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-9ec42"))
						Expect(loadbalancer.Internal).Should(Equal(false))
						Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
						Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L7_PACKAGE_ID))
						Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 7"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

						// check pool
						pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect(len(pools.Items)).Should(Equal(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(BeElementOf(
								"vks-9ec42-default-test-service-gogsf-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
							Expect(pool.Protocol).Should(Equal("HTTP"))
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
							for _, member := range pool.Members.Items {
								Expect(member.ProtocolPort).Should(Equal(30000))
								Expect(member.MonitorPort).Should(Equal(30000))
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
						Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
						for _, listener := range listeners.Items {
							Expect(listener.Protocol).Should(Equal("HTTP"))
							Expect(listener.ProtocolPort).Should(Equal(80))
							Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
							Expect(listener.DefaultPoolId).Should(Equal(""))   // no default pool
							Expect(listener.DefaultPoolName).Should(Equal("")) // no default pool
							Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
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

							// check policy
							policies, err := mockProvider.ListPolicyOfListener(ctx, loadbalancer.UUID, listener.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(policies).ShouldNot(BeNil())
							Expect(len(policies.Items)).Should(Equal(1)) // number of policy
							for _, policy := range policies.Items {
								Expect(policy.Name).Should(Equal("vks-9ec42-false-r0-p0"))
								Expect(policy.Action).Should(Equal(string(loadbalancerv2.PolicyActionREDIRECTTOPOOL)))
							}
						}
					},
					steps: []stepType{
						{
							name: "update rule to new service port",
							updateObjects: func() []client.Object {
								object := networkingv1.Ingress{}
								Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, &object)).Should(Succeed())
								object.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number = 443
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(20 * time.Second)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-test-clust-default-test-servi-9ec42"))
								Expect(loadbalancer.Internal).Should(Equal(false))
								Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
								Expect(loadbalancer.PackageID).Should(Equal(consts.DEFAULT_L7_PACKAGE_ID))
								Expect(loadbalancer.SubnetID).Should(Equal(mockProvider.GetSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 7"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(mockProvider.GetSubnetCIDR()))

								// check pool
								pools, err := mockProvider.ListPool(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(pools).ShouldNot(BeNil())
								Expect(len(pools.Items)).Should(Equal(1)) // number of pool
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(BeElementOf(
										"vks-9ec42-default-test-service-gogsf-443"))
									Expect(pool.Description).Should(Equal("????????"))
									Expect(pool.Status).Should(Equal("ACTIVE"))
									Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
									Expect(pool.Protocol).Should(Equal("HTTP"))
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
									for _, member := range pool.Members.Items {
										Expect(member.ProtocolPort).Should(Equal(30001))
										Expect(member.MonitorPort).Should(Equal(30001))
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
								Expect(len(listeners.Items)).Should(Equal(1)) // number of listener
								for _, listener := range listeners.Items {
									Expect(listener.Protocol).Should(Equal("HTTP"))
									Expect(listener.ProtocolPort).Should(Equal(80))
									Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
									Expect(listener.DefaultPoolId).Should(Equal(""))   // no default pool
									Expect(listener.DefaultPoolName).Should(Equal("")) // no default pool
									Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
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

									// check policy
									policies, err := mockProvider.ListPolicyOfListener(ctx, loadbalancer.UUID, listener.UUID)
									Expect(err).ShouldNot(HaveOccurred())
									Expect(policies).ShouldNot(BeNil())
									Expect(len(policies.Items)).Should(Equal(1)) // number of policy
									for _, policy := range policies.Items {
										Expect(policy.Name).Should(Equal("vks-9ec42-false-r0-p0"))
										Expect(policy.Action).Should(Equal(string(loadbalancerv2.PolicyActionREDIRECTTOPOOL)))
									}
								}
							},
						},
					},
				},
			}

			for _, tt := range tests {
				logrus.Info("------------------- ", tt.name, " -------------------")
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
					getObj := &networkingv1.Ingress{}
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
				Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
				Eventually(func() bool {
					getObj := &networkingv1.Ingress{}
					err := k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, getObj)
					return err != nil
				}, 2*timeout, interval).Should(BeTrue())
				_, err = mockProvider.GetLoadBalancerByID(ctx, loadbalancerID)
				Expect(err).Should(HaveOccurred())

				for _, depend := range depends {
					Expect(k8sClient.Delete(ctx, depend)).Should(Succeed())
					err := k8sClient.Get(ctx, client.ObjectKey{Name: depend.GetName(), Namespace: depend.GetNamespace()}, depend)
					Expect(err).Should(HaveOccurred())
				}
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

func newIngressResource(name, namespace string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"annd2.space": "test",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: PointerOf("vngcloud"),
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "kubernetes",
					Port: networkingv1.ServiceBackendPort{
						Number: 443,
					},
				},
			},
		},
	}
}

func updateIngressAnnotation(name, namespace, key, value string) {
	ingress := &networkingv1.Ingress{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
	if value == "-" {
		delete(ingress.Annotations, key)
	} else {
		ingress.Annotations[key] = value
	}
	Expect(k8sClient.Update(ctx, ingress)).Should(Succeed())

	if value == "-" {
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
			return ingress != nil && ingress.Annotations[key] == ""
		}, timeout, interval).Should(BeTrue())
	} else {
		Eventually(func() bool {
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, ingress)).Should(Succeed())
			return ingress != nil && ingress.Annotations[key] == value
		}, timeout, interval).Should(BeTrue())
	}
}

func newServiceNodePortResource(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
}

func newEnpointResource(name, namespace string) *corev1.Endpoints {
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
