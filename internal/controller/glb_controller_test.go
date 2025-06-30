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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/provider"
)

const (
	skipVGLBTest = false
)

var _ = Describe("VngcloudGlobalLoadBalancer Controller", func() {

	Context("When create vglb with specific annotation", func() {
		It("created load balancer shoud have specific attribute", func() {
			if skipVGLBTest {
				Skip("Skip test")
			}

			testss := []TestType[*v1alpha1.VngcloudGlobalLoadBalancer]{
				{
					name: "create with default annotation",
					generateDepends: func() []client.Object {
						service := newServiceNodePortResource("test-service-gogsf", "default")
						service.Spec.Ports = []corev1.ServicePort{
							{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
						}
						return []client.Object{service}
					},
					generateObj: func() []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer] {
						vglb := newVGLBResource("test-service-gogsf", "default", map[string]string{}, map[string]string{})
						Expect(vglb).NotTo(BeNil())
						return []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer]{{obj: vglb, expect: func() {}}}
					},
					expect: func() {
						// wait until reconcile done
						time.Sleep(timeWaitRecocile)

						// expect nothing happen .....................

					},
				},
			}

			for _, tt := range testss {
				logrus.Info("Running test: ", tt.name)
				RunMultiStepTest(tt)
			}
		})
	})

	Context("Create with minimum config", func() {
		It("should create successfully", func() {
			if skipVGLBTest {
				Skip("Skip test")
			}

			test := TestType[*v1alpha1.VngcloudGlobalLoadBalancer]{
				preTest:  func() {},
				postTest: func() {},
				name:     "Create with minimum config",
				generateDepends: func() []client.Object {
					// create service node port
					service := newServiceNodePortResource("test-service-gogsf", "default")
					service.Spec.Ports = []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
					}
					return []client.Object{service}
				},
				generateObj: func() []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer] {
					vglb := newVGLBResource("test-service-gogsf", "default",
						map[string]string{
							"fleet.vngcloud.vn/config-cluster-id": mockClusterID,
							"fleet.vngcloud.vn/active-clusters":   mockClusterID,
						},
						map[string]string{
							"fleet.vngcloud.vn/fleet-id": "vf-123456",
						})
					Expect(vglb).NotTo(BeNil())
					return []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer]{{obj: vglb, expect: func() {}}}
				},
				expect: func() {
					// wait until reconcile done
					time.Sleep(3 * timeWaitRecocile)

					// get load balancer by id in resource annotation
					obj := &v1alpha1.VngcloudGlobalLoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "test-service-gogsf", Namespace: "default"}}
					loadbalancer := getGLBByAnnotation(k8sClient, obj)
					Expect(loadbalancer).ShouldNot(BeNil())

					// check pool
					pools, err := mockProvider.ListGlobalPools(ctx, loadbalancer.ID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(pools).ShouldNot(BeNil())
					Expect((pools.Items)).Should(HaveLen(1)) // number of pool
					for _, pool := range pools.Items {
						Expect(pool.Name).Should(BeElementOf(
							"vks-vf-123456-default-test-serv-a9dfa-TCP-80",
						))
						Expect(pool.Algorithm).Should(Equal("ROUND_ROBIN"))
						Expect(pool.Protocol).Should(Equal("TCP"))

						poolMembers, err := mockProvider.ListGlobalPoolMembers(ctx, loadbalancer.ID, pool.ID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(poolMembers).ShouldNot(BeNil())
						Expect((poolMembers.Items)).Should(HaveLen(1)) // number of member in pool = number of node or number of endpoint
						for _, pMember := range poolMembers.Items {
							Expect(pMember.VpcID).Should(Equal(provider.MockNetID))
							Expect(pMember.TrafficDial).Should(Equal(100))
							Expect(pMember.Region).Should(Equal("hcm"))
							Expect(pMember.Name).Should(Equal("hcm-netID"))
							Expect(pMember.Members.Items).Should(HaveLen(4))
							for _, member := range pMember.Members.Items {
								Expect(member.Name).Should(Equal(mockClusterID))
								Expect(member.GlobalLoadBalancerID).Should(Equal(loadbalancer.ID))
								Expect(member.GlobalPoolMemberID).Should(Equal(pMember.ID))
								Expect(member.SubnetID).Should(Equal(provider.MockSubnetID))
								Expect(member.Weight).Should(Equal(1))
								Expect(member.Port).Should(Equal(30000))
								Expect(member.MonitorPort).Should(Equal(30000))
								Expect(member.BackupRole).Should(Equal(false))
								Expect(member.Address).Should(BeElementOf(
									mockNode1.Status.Addresses[0].Address,
									mockNode2.Status.Addresses[0].Address,
									mockNode3.Status.Addresses[0].Address,
									mockNode4.Status.Addresses[0].Address,
								))
							}
						}
					}

					// check listener
					listeners, err := mockProvider.ListGlobalListeners(ctx, loadbalancer.ID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(listeners).ShouldNot(BeNil())
					Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
					for _, listener := range listeners.Items {
						Expect(listener.Name).Should(BeElementOf(
							"vks-vf-123456-default-test-serv-a9dfa-TCP-80",
						))
						Expect(listener.Protocol).Should(Equal("TCP"))
						Expect(listener.Port).Should(Equal(80))
						Expect(listener.GlobalPoolID).Should(Equal(pools.Items[0].ID))
					}
				},
				steps: []StepType{
					{
						kindStep: updateStep,
						name:     "update service to have 2 ports, expect 2 pools and 2 listeners",
						getObject: func() client.Object {
							service := newServiceNodePortResource("test-service-gogsf", "default")
							service.Spec.Ports = []corev1.ServicePort{
								{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
								{Name: "https", Port: 443, TargetPort: intstr.FromInt(443), Protocol: corev1.ProtocolTCP, NodePort: 30001},
							}
							return service
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(3 * timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &v1alpha1.VngcloudGlobalLoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "test-service-gogsf", Namespace: "default"}}
							loadbalancer := getGLBByAnnotation(k8sClient, obj)
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListGlobalPools(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(2)) // number of pool
							for _, pool := range pools.Items {
								Expect(pool.Name).Should(BeElementOf(
									"vks-vf-123456-default-test-serv-a9dfa-TCP-80",
									"vks-vf-123456-default-test-serv-a9dfa-TCP-443",
								))
								Expect(pool.Algorithm).Should(Equal("ROUND_ROBIN"))
								Expect(pool.Protocol).Should(Equal("TCP"))

								poolMembers, err := mockProvider.ListGlobalPoolMembers(ctx, loadbalancer.ID, pool.ID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(poolMembers).ShouldNot(BeNil())
								Expect((poolMembers.Items)).Should(HaveLen(1)) // number of member in pool = number of node or number of endpoint
								for _, pMember := range poolMembers.Items {
									Expect(pMember.VpcID).Should(Equal(provider.MockNetID))
									Expect(pMember.TrafficDial).Should(Equal(100))
									Expect(pMember.Region).Should(Equal("hcm"))
									Expect(pMember.Name).Should(Equal("hcm-netID"))
									Expect(pMember.Members.Items).Should(HaveLen(4))
									for _, member := range pMember.Members.Items {
										Expect(member.Name).Should(Equal(mockClusterID))
										Expect(member.GlobalLoadBalancerID).Should(Equal(loadbalancer.ID))
										Expect(member.GlobalPoolMemberID).Should(Equal(pMember.ID))
										Expect(member.SubnetID).Should(Equal(provider.MockSubnetID))
										Expect(member.Weight).Should(Equal(1))
										Expect(member.Port).Should(BeElementOf(30000, 30001))
										Expect(member.MonitorPort).Should(BeElementOf(30000, 30001))
										Expect(member.BackupRole).Should(Equal(false))
										Expect(member.Address).Should(BeElementOf(
											mockNode1.Status.Addresses[0].Address,
											mockNode2.Status.Addresses[0].Address,
											mockNode3.Status.Addresses[0].Address,
											mockNode4.Status.Addresses[0].Address,
										))
									}
								}
							}

							// check listener
							listeners, err := mockProvider.ListGlobalListeners(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(listeners).ShouldNot(BeNil())
							Expect((listeners.Items)).Should(HaveLen(2)) // number of listener
							for _, listener := range listeners.Items {
								Expect(listener.Name).Should(BeElementOf(
									"vks-vf-123456-default-test-serv-a9dfa-TCP-80",
									"vks-vf-123456-default-test-serv-a9dfa-TCP-443",
								))
								Expect(listener.Protocol).Should(Equal("TCP"))
								Expect(listener.Port).Should(BeElementOf(80, 443))
								Expect(listener.GlobalPoolID).Should(BeElementOf(pools.Items[0].ID, pools.Items[1].ID))
							}
						},
					},
					{
						kindStep: updateStep,
						name:     "update service to have 1 ports, expect 1 pools and 1 listeners",
						getObject: func() client.Object {
							service := newServiceNodePortResource("test-service-gogsf", "default")
							service.Spec.Ports = []corev1.ServicePort{
								{Name: "https", Port: 443, TargetPort: intstr.FromInt(443), Protocol: corev1.ProtocolTCP, NodePort: 30001},
							}
							return service
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(3 * timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &v1alpha1.VngcloudGlobalLoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "test-service-gogsf", Namespace: "default"}}
							loadbalancer := getGLBByAnnotation(k8sClient, obj)
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListGlobalPools(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(1)) // number of pool
							for _, pool := range pools.Items {

								if pool.Name == "vks-vf-123456-default-test-serv-a9dfa-TCP-80" {
									poolMembers, err := mockProvider.ListGlobalPoolMembers(ctx, loadbalancer.ID, pool.ID)
									Expect(err).ShouldNot(HaveOccurred())
									Expect(poolMembers).ShouldNot(BeNil())

									Expect((poolMembers.Items)).Should(HaveLen(1))
									for _, pMember := range poolMembers.Items {
										Expect(pMember.Members.Items).Should(HaveLen(0)) // it delete old member
									}
								}

								if pool.Name == "vks-vf-123456-default-test-serv-a9dfa-TCP-443" {
									poolMembers, err := mockProvider.ListGlobalPoolMembers(ctx, loadbalancer.ID, pool.ID)
									Expect(err).ShouldNot(HaveOccurred())
									Expect(poolMembers).ShouldNot(BeNil())

									Expect((poolMembers.Items)).Should(HaveLen(1))
									for _, pMember := range poolMembers.Items {
										Expect(pMember.Members.Items).Should(HaveLen(4))
									}
								}
							}

							// check listener
							listeners, err := mockProvider.ListGlobalListeners(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(listeners).ShouldNot(BeNil())
							Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
							for _, listener := range listeners.Items {
								Expect(listener.Name).Should(BeElementOf(
									"vks-vf-123456-default-test-serv-a9dfa-TCP-443",
								))
								Expect(listener.Protocol).Should(Equal("TCP"))
								Expect(listener.Port).Should(BeElementOf(443))
								Expect(listener.GlobalPoolID).Should(BeElementOf(pools.Items[0].ID))
							}
						},
					},
				},
				expectAfterDelete: func() {},
			}

			logrus.Info("Running test: ", test.name)
			RunMultiStepTest(test)
		})
	})

	Context("Create with minimum config, then switch to member cluster to test it behavior", func() {
		It("should create successfully", func() {
			if skipVGLBTest {
				Skip("Skip test")
			}

			test := TestType[*v1alpha1.VngcloudGlobalLoadBalancer]{
				preTest:  func() {},
				postTest: func() {},
				name:     "Create with minimum config",
				generateDepends: func() []client.Object {
					// create service node port
					service := newServiceNodePortResource("test-service-gogsf", "default")
					service.Spec.Ports = []corev1.ServicePort{
						{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
					}
					return []client.Object{service}
				},
				generateObj: func() []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer] {
					vglb := newVGLBResource("test-service-gogsf", "default",
						map[string]string{
							"fleet.vngcloud.vn/config-cluster-id": mockClusterID,
							"fleet.vngcloud.vn/active-clusters":   mockClusterID,
						},
						map[string]string{
							"fleet.vngcloud.vn/fleet-id": "vf-123456",
						})
					Expect(vglb).NotTo(BeNil())
					return []ObjectAndExpect[*v1alpha1.VngcloudGlobalLoadBalancer]{{obj: vglb, expect: func() {}}}
				},
				expect: func() {
					// wait until reconcile done
					time.Sleep(3 * timeWaitRecocile)

					// already check in above test
				},
				steps: []StepType{
					{
						kindStep: updateStep,
						name:     "update the config cluster annotation to another cluster",
						getObject: func() client.Object {
							vglb := &v1alpha1.VngcloudGlobalLoadBalancer{}
							Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-service-gogsf", Namespace: "default"}, vglb)).Should(Succeed())
							vglb.Annotations["fleet.vngcloud.vn/config-cluster-id"] = "new-cluster-id"
							return vglb
						},
						expect: func() {
							// wait until reconcile done
							time.Sleep(3 * timeWaitRecocile)

							// get load balancer by id in resource annotation
							obj := &v1alpha1.VngcloudGlobalLoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "test-service-gogsf", Namespace: "default"}}
							loadbalancer := getGLBByAnnotation(k8sClient, obj)
							Expect(loadbalancer).ShouldNot(BeNil())

							// check pool
							pools, err := mockProvider.ListGlobalPools(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(pools).ShouldNot(BeNil())
							Expect((pools.Items)).Should(HaveLen(1)) // number of pool
							for _, pool := range pools.Items {
								poolMembers, err := mockProvider.ListGlobalPoolMembers(ctx, loadbalancer.ID, pool.ID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(poolMembers).ShouldNot(BeNil())
								Expect((poolMembers.Items)).Should(HaveLen(1)) // number of member in pool = number of node or number of endpoint
							}

							// check listener
							listeners, err := mockProvider.ListGlobalListeners(ctx, loadbalancer.ID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(listeners).ShouldNot(BeNil())
							Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
						},
					},
				},
				expectAfterDelete: func() {},
			}

			logrus.Info("Running test: ", test.name)
			RunMultiStepTest(test)
		})
	})
})

func newVGLBResource(name, namespace string, annotations, labels map[string]string) *v1alpha1.VngcloudGlobalLoadBalancer {
	return &v1alpha1.VngcloudGlobalLoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
			Labels:      labels,
		},
	}
}
