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

package networking

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

const (
	timeout  = time.Second * 5
	interval = time.Millisecond * 250
)

var _ = Describe("Ingress Controller", func() {

	AfterEach(func() {
		// Ensure clean state before each test
		expectNoLoadBalancers()
		expectNoSecurityGroups()
		expectNoIngresses()
		expectNoServices()
		expectNoLBCs()
		expectNoNSGs()
		expectNoEndpoints()
	})

	Context("When create ingress with default annotation", func() {
		It("created load balancer shoud have specific attribute", func() {

			serviceName := "test-service-gogsf"
			namespace := "default"
			ingressName := "test-service-gogsf"

			// Create endpoint
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create Service
			service := newServiceNodePortResource(serviceName, namespace)
			service.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Create Ingress
			ingress := newIngressResource(ingressName, namespace)
			Expect(ingress).NotTo(BeNil())
			ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: serviceName,
					Port: networkingv1.ServiceBackendPort{
						Number: 80,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

			// Verify LoadBalancer was created in mock repo
			Eventually(func(g Gomega) {
				lbcList, err := listLbcByIngress(ingressName, namespace)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(len(lbcList.Items)).Should(Equal(1))

				lbc := &lbcList.Items[0]
				g.Expect(lbc.Status.LoadBalancerId).ShouldNot(BeNil())
				loadbalancerId := *lbc.Status.LoadBalancerId

				loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(loadbalancer).ShouldNot(BeNil())
				g.Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-bea48"))
				g.Expect(loadbalancer.LoadBalancerSchema).Should(Equal(mockConfig.LoadBalancerOpts.DefaultScheme))
				g.Expect(loadbalancer.PackageID).Should(Equal(vngcloud_mocks.MockL7PackageId))
				g.Expect(loadbalancer.SubnetID).Should(BeElementOf(vngcloud_mocks.NodeSubnetIDs))
				g.Expect(loadbalancer.ZoneID).Should(Equal(vngcloud_mocks.MapSubnetToZone[loadbalancer.SubnetID]))
				g.Expect(loadbalancer.Type).Should(Equal(string(loadbalancerv2.LoadBalancerTypeLayer7)))
				g.Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloud_mocks.MapSubnetToCIDR[loadbalancer.SubnetID]))

				// check pool
				pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(pools).ShouldNot(BeNil())
				g.Expect((pools.Items)).Should(HaveLen(1)) // number of pool
				for _, pool := range pools.Items {
					g.Expect(pool.Name).Should(BeElementOf(
						consts.DEFAULT_NAME_DEFAULT_POOL,
						"vks-k8s-000000-default-test-serv-fbaa0-TCP-80"))
					g.Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
					g.Expect(pool.Protocol).Should(Equal("HTTP"))
					g.Expect(pool.Stickiness).Should(BeFalse())
					g.Expect(pool.TLSEncryption).Should(BeFalse())

					g.Expect(pool.HealthMonitor).ShouldNot(BeNil())
					g.Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
					g.Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
					g.Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
					g.Expect(pool.HealthMonitor.Interval).Should(Equal(30))
					g.Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

					g.Expect(pool.Members).ShouldNot(BeNil())
					g.Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
					for _, member := range pool.Members.Items {
						g.Expect(member.ProtocolPort).Should(Equal(30000))
						g.Expect(member.MonitorPort).Should(Equal(30000))
						g.Expect(member.Address).Should(BeElementOf(
							vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
							vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
							vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
							vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
					}
				}

				// check listener
				listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(listeners).ShouldNot(BeNil())
				g.Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
				for _, listener := range listeners.Items {
					g.Expect(listener.Protocol).Should(Equal("HTTP"))
					g.Expect(listener.ProtocolPort).Should(Equal(80))
					g.Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
					g.Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
					g.Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
					g.Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
					g.Expect(listener.TimeoutClient).Should(Equal(50))
					g.Expect(listener.TimeoutConnection).Should(Equal(5))
					g.Expect(listener.TimeoutMember).Should(Equal(50))
				}

				listNsg, err := listNsgByIngress(ingressName, namespace)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(len(listNsg.Items)).Should(Equal(1))

				nsg := &listNsg.Items[0]
				g.Expect(nsg.Status.ManagedSecurityGroup).ShouldNot(BeNil())
				g.Expect(nsg.Status.ManagedSecurityGroup.Id).ShouldNot(BeNil())

				secgroupId := *nsg.Status.ManagedSecurityGroup.Id
				secgroup, err := vngcloudRepo.GetSecurityGroup(ctx, secgroupId)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(secgroup).ShouldNot(BeNil())
				g.Expect(secgroup.Name).Should(Equal(nsg.Spec.ManagedSecurityGroup.Name))
			}, timeout, interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
		})
	})

	// Context("When create ingress without default backend, only 1 rule", func() {
	// 	It("should create load balancer with correct attributes and update when rule changes", func() {
	// 		Skip("skip")
	// 		serviceName := "test-service-gogsf"
	// 		namespace := "default"
	// 		ingressName := "test-service-gogsf"

	// 		// Create endpoint
	// 		endpoint := newEndpointResource(serviceName, namespace)
	// 		endpoint.Subsets = []corev1.EndpointSubset{
	// 			{
	// 				Addresses: []corev1.EndpointAddress{
	// 					{IP: vngcloud_mocks.MockNode1.Status.Addresses[0].Address},
	// 					{IP: vngcloud_mocks.MockNode2.Status.Addresses[0].Address},
	// 					{IP: vngcloud_mocks.MockNode3.Status.Addresses[0].Address},
	// 					{IP: vngcloud_mocks.MockNode4.Status.Addresses[0].Address},
	// 				},
	// 				Ports: []corev1.EndpointPort{
	// 					{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
	// 					{Name: "https", Port: 81, Protocol: corev1.ProtocolTCP},
	// 				},
	// 			},
	// 		}
	// 		Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

	// 		// Create Service
	// 		service := newServiceNodePortResource(serviceName, namespace)
	// 		service.Spec.Ports = []corev1.ServicePort{
	// 			{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
	// 			{Name: "https", Port: 443, TargetPort: intstr.FromInt(81), Protocol: corev1.ProtocolTCP, NodePort: 30001},
	// 		}
	// 		Expect(k8sClient.Create(ctx, service)).Should(Succeed())

	// 		// Create Ingress
	// 		ingress := newIngressResource(ingressName, namespace)
	// 		Expect(ingress).NotTo(BeNil())
	// 		ingress.Spec.DefaultBackend = nil
	// 		ingress.Spec.Rules = []networkingv1.IngressRule{
	// 			{
	// 				Host: "test.com",
	// 				IngressRuleValue: networkingv1.IngressRuleValue{
	// 					HTTP: &networkingv1.HTTPIngressRuleValue{
	// 						Paths: []networkingv1.HTTPIngressPath{
	// 							{
	// 								PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
	// 								Path:     "/",
	// 								Backend: networkingv1.IngressBackend{
	// 									Service: &networkingv1.IngressServiceBackend{
	// 										Name: serviceName,
	// 										Port: networkingv1.ServiceBackendPort{Number: 80},
	// 									},
	// 								},
	// 							},
	// 						},
	// 					},
	// 				},
	// 			},
	// 		}
	// 		Expect(k8sClient.Create(ctx, ingress)).Should(Succeed())

	// 		// Wait for LBC to be created
	// 		var lbcList *v1alpha1.LoadBalancerConfigList
	// 		Eventually(func() int {
	// 			list, err := listLbcByIngress(ingressName, namespace)
	// 			if err != nil {
	// 				return -1
	// 			}
	// 			lbcList = list
	// 			return len(list.Items)
	// 		}, timeout*2, interval).Should(Equal(1))

	// 		lbc := &lbcList.Items[0]

	// 		// Wait for LoadBalancer ID in LBC status
	// 		loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
	// 		Expect(err).ShouldNot(HaveOccurred())
	// 		Expect(loadbalancerId).ShouldNot(BeEmpty())

	// 		// Verify LoadBalancer was created in mock repo
	// 		Eventually(func(g Gomega) {
	// 			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(loadbalancer).ShouldNot(BeNil())
	// 			g.Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-bea48"))
	// 			g.Expect(loadbalancer.LoadBalancerSchema).Should(Equal(mockConfig.LoadBalancerOpts.DefaultScheme))
	// 			// g.Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L7_PACKAGE_ID)) // TODO: fix me
	// 			// g.Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID())) // TODO: fix me
	// 			g.Expect(loadbalancer.Type).Should(Equal("Layer 7"))

	// 			// check pool
	// 			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(pools).ShouldNot(BeNil())
	// 			g.Expect((pools.Items)).Should(HaveLen(1)) // number of pool
	// 			for _, pool := range pools.Items {
	// 				g.Expect(pool.Name).Should(BeElementOf(
	// 					"vks-bea48-default-test-service-gogsf-80"))
	// 				g.Expect(pool.Status).Should(Equal("ACTIVE"))
	// 				g.Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
	// 				g.Expect(pool.Protocol).Should(Equal("HTTP"))
	// 				g.Expect(pool.Stickiness).Should(BeFalse())
	// 				g.Expect(pool.TLSEncryption).Should(BeFalse())

	// 				g.Expect(pool.HealthMonitor).ShouldNot(BeNil())
	// 				g.Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
	// 				g.Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
	// 				g.Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
	// 				g.Expect(pool.HealthMonitor.Interval).Should(Equal(30))
	// 				g.Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

	// 				g.Expect(pool.Members).ShouldNot(BeNil())
	// 				g.Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
	// 				for _, member := range pool.Members.Items {
	// 					g.Expect(member.ProtocolPort).Should(Equal(30000))
	// 					g.Expect(member.MonitorPort).Should(Equal(30000))
	// 					g.Expect(member.Address).Should(BeElementOf(
	// 						vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
	// 				}
	// 			}

	// 			// check listener
	// 			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(listeners).ShouldNot(BeNil())
	// 			g.Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
	// 			for _, listener := range listeners.Items {
	// 				g.Expect(listener.Protocol).Should(Equal("HTTP"))
	// 				g.Expect(listener.ProtocolPort).Should(Equal(80))
	// 				g.Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
	// 				g.Expect(listener.DefaultPoolId).Should(Equal(""))   // no default pool
	// 				g.Expect(listener.DefaultPoolName).Should(Equal("")) // no default pool
	// 				g.Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
	// 				g.Expect(listener.TimeoutClient).Should(Equal(50))
	// 				g.Expect(listener.TimeoutConnection).Should(Equal(5))
	// 				g.Expect(listener.TimeoutMember).Should(Equal(50))
	// 				g.Expect(listener.Description).Should(Equal("????????"))

	// 				// check policy
	// 				policies, err := vngcloudRepo.ListPolicyOfListener(ctx, loadbalancer.UUID, listener.UUID)
	// 				g.Expect(err).ShouldNot(HaveOccurred())
	// 				g.Expect(policies).ShouldNot(BeNil())
	// 				g.Expect((policies.Items)).Should(HaveLen(1)) // number of policy
	// 				for _, policy := range policies.Items {
	// 					g.Expect(policy.Name).Should(Equal("vks-bea48-false-r0-p0"))
	// 					g.Expect(policy.Action).Should(Equal(string(loadbalancerv2.PolicyActionREDIRECTTOPOOL)))
	// 				}
	// 			}
	// 		}, timeout*2, interval).Should(Succeed())

	// 		// Step: update rule to new service port
	// 		object := &networkingv1.Ingress{}
	// 		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ingressName, Namespace: namespace}, object)).Should(Succeed())
	// 		object.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number = 443
	// 		Expect(k8sClient.Update(ctx, object)).Should(Succeed())

	// 		// Verify LoadBalancer after update
	// 		Eventually(func(g Gomega) {
	// 			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(loadbalancer).ShouldNot(BeNil())
	// 			g.Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-bea48"))
	// 			g.Expect(loadbalancer.LoadBalancerSchema).Should(Equal(mockConfig.LoadBalancerOpts.DefaultScheme))
	// 			// g.Expect(loadbalancer.PackageID).Should(Equal(provider.DEFAULT_L7_PACKAGE_ID)) // TODO: fix me
	// 			// g.Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID())) // TODO: fix me
	// 			g.Expect(loadbalancer.Type).Should(Equal("Layer 7"))

	// 			// check pool after update
	// 			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(pools).ShouldNot(BeNil())
	// 			g.Expect((pools.Items)).Should(HaveLen(1)) // number of pool
	// 			for _, pool := range pools.Items {
	// 				g.Expect(pool.Name).Should(BeElementOf(
	// 					"vks-bea48-default-test-service-gogsf-443"))
	// 				g.Expect(pool.Status).Should(Equal("ACTIVE"))
	// 				g.Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
	// 				g.Expect(pool.Protocol).Should(Equal("HTTP"))
	// 				g.Expect(pool.Stickiness).Should(BeFalse())
	// 				g.Expect(pool.TLSEncryption).Should(BeFalse())

	// 				g.Expect(pool.HealthMonitor).ShouldNot(BeNil())
	// 				g.Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
	// 				g.Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(3))
	// 				g.Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(3))
	// 				g.Expect(pool.HealthMonitor.Interval).Should(Equal(30))
	// 				g.Expect(pool.HealthMonitor.Timeout).Should(Equal(5))

	// 				g.Expect(pool.Members).ShouldNot(BeNil())
	// 				g.Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
	// 				for _, member := range pool.Members.Items {
	// 					g.Expect(member.ProtocolPort).Should(Equal(30001))
	// 					g.Expect(member.MonitorPort).Should(Equal(30001))
	// 					g.Expect(member.Address).Should(BeElementOf(
	// 						vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
	// 						vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
	// 				}
	// 			}

	// 			// check listener after update
	// 			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
	// 			g.Expect(err).ShouldNot(HaveOccurred())
	// 			g.Expect(listeners).ShouldNot(BeNil())
	// 			g.Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
	// 			for _, listener := range listeners.Items {
	// 				g.Expect(listener.Protocol).Should(Equal("HTTP"))
	// 				g.Expect(listener.ProtocolPort).Should(Equal(80))
	// 				g.Expect(listener.AllowedCidrs).Should(Equal("0.0.0.0/0"))
	// 				g.Expect(listener.DefaultPoolId).Should(Equal(""))   // no default pool
	// 				g.Expect(listener.DefaultPoolName).Should(Equal("")) // no default pool
	// 				g.Expect(listener.Name).Should(Equal(consts.DEFAULT_HTTP_LISTENER_NAME))
	// 				g.Expect(listener.TimeoutClient).Should(Equal(50))
	// 				g.Expect(listener.TimeoutConnection).Should(Equal(5))
	// 				g.Expect(listener.TimeoutMember).Should(Equal(50))

	// 				// check policy
	// 				policies, err := vngcloudRepo.ListPolicyOfListener(ctx, loadbalancer.UUID, listener.UUID)
	// 				g.Expect(err).ShouldNot(HaveOccurred())
	// 				g.Expect(policies).ShouldNot(BeNil())
	// 				g.Expect((policies.Items)).Should(HaveLen(1)) // number of policy
	// 				for _, policy := range policies.Items {
	// 					g.Expect(policy.Name).Should(Equal("vks-bea48-false-r0-p0"))
	// 					g.Expect(policy.Action).Should(Equal(string(loadbalancerv2.PolicyActionREDIRECTTOPOOL)))
	// 				}
	// 			}
	// 		}, timeout*2, interval).Should(Succeed())

	// 		// Cleanup
	// 		Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
	// 		Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
	// 		Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
	// 	})
	// })

})
