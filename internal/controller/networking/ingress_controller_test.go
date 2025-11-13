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

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
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

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForIngress(ingressName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Verify LBC spec
			Expect(lbc.Spec.Type).Should(Equal(loadbalancerv2.LoadBalancerTypeLayer7))

			// Wait for LoadBalancer ID in LBC status
			loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancerId).ShouldNot(BeEmpty())

			// Verify LoadBalancer was created in mock repo
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())
			Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-bea48"))
			// Expect(loadbalancer.Internal).Should(BeFalse()) // TODO: fix me
			Expect(loadbalancer.LoadBalancerSchema).Should(Equal(mockConfig.LoadBalancerOpts.DefaultScheme))
			// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL7PackageName)) // TODO: fix me
			// Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID())) // TODO: fix me
			Expect(loadbalancer.Type).Should(Equal(string(loadbalancerv2.LoadBalancerTypeLayer7)))
			// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR())) // TODO: fix me

			// check pool
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect((pools.Items)).Should(HaveLen(1)) // number of pool
			for _, pool := range pools.Items {
				Expect(pool.Name).Should(BeElementOf(
					consts.DEFAULT_NAME_DEFAULT_POOL,
					"vks-k8s-000000-default-test-serv-fbaa0-TCP-80"))
				Expect(pool.Description).Should(Equal("????????"))
				Expect(pool.Status).Should(Equal("ACTIVE"))
				Expect(pool.LoadBalanceMethod).Should(Equal("ROUND_ROBIN"))
				Expect(pool.Protocol).Should(Equal("HTTP"))
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
			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
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
			}

			// Verify Security Group was created
			secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(secgroups).ShouldNot(BeNil())
			Expect(len(secgroups.Items)).Should(BeNumerically(">", 0))

			// Cleanup
			Expect(k8sClient.Delete(ctx, ingress)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
		})
	})

})
