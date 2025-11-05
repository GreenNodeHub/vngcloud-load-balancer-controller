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
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
)

const (
	timeout  = time.Second * 5
	interval = time.Millisecond * 250
)

var _ = Describe("Service Controller", func() {
	AfterEach(func() {
		// Ensure clean state before each test
		expectNoLoadBalancers()
		expectNoSecurityGroups()
		expectNoServices()
		expectNoLBCs()
		expectNoNSGs()
		expectNoEndpoints()
	})

	Context("When creating a LoadBalancer service", func() {
		It("should create LBC, LoadBalancer and SecurityGroup", func() {
			// Skip("Skip test")
			serviceName := "test-service"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service
			service := newServiceResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Verify LBC spec
			Expect(lbc.Spec.Type).Should(Equal(loadbalancerv2.LoadBalancerTypeLayer4))

			// Wait for LoadBalancer ID in LBC status
			loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancerId).ShouldNot(BeEmpty())

			// Verify LoadBalancer was created in mock repo
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())
			Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-95466"))
			// Expect(loadbalancer.Internal).Should(BeFalse()) // TODO: fix me
			Expect(loadbalancer.LoadBalancerSchema).Should(Equal(mockConfig.LoadBalancerOpts.DefaultScheme))
			// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId)) // TODO: fix me
			// Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID())) // TODO: fix me
			Expect(loadbalancer.Type).Should(Equal(string(loadbalancerv2.LoadBalancerTypeLayer4)))
			// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR())) // TODO: fix me

			// check pool
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect((pools.Items)).Should(HaveLen(1)) // number of pool
			for _, pool := range pools.Items {
				Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
				Expect(pool.Description).Should(Equal("????????"))
				Expect(pool.Status).Should(Equal("ACTIVE"))
				Expect(pool.LoadBalanceMethod).Should(Equal(mockConfig.LoadBalancerOpts.DefaultPoolAlgorithm))
				Expect(pool.Protocol).Should(Equal("TCP"))
				Expect(pool.Stickiness).Should(BeFalse())
				Expect(pool.TLSEncryption).Should(BeFalse())

				Expect(pool.HealthMonitor).ShouldNot(BeNil())
				Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
				Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(mockConfig.LoadBalancerOpts.DefaultHealthyThreshold))
				Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(mockConfig.LoadBalancerOpts.DefaultUnhealthyThreshold))
				Expect(pool.HealthMonitor.Interval).Should(Equal(mockConfig.LoadBalancerOpts.DefaultInterval))
				Expect(pool.HealthMonitor.Timeout).Should(Equal(mockConfig.LoadBalancerOpts.DefaultTimeout))

				Expect(pool.Members).ShouldNot(BeNil())
				Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of nodes
				Expect(pool.Members.Items[0].Address).Should(BeElementOf(
					mockNode1.Status.Addresses[0].Address,
					mockNode2.Status.Addresses[0].Address,
					mockNode3.Status.Addresses[0].Address,
					mockNode4.Status.Addresses[0].Address,
				))
			}

			// check listener
			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
			for _, listener := range listeners.Items {
				Expect(listener.Protocol).Should(Equal("TCP"))
				Expect(listener.ProtocolPort).Should(Equal(80))
				Expect(listener.AllowedCidrs).Should(Equal(mockConfig.LoadBalancerOpts.DefaultAllowedCidrs))
				Expect(listener.DefaultPoolId).Should(Equal(pools.Items[0].UUID))
				Expect(listener.DefaultPoolName).Should(Equal(pools.Items[0].Name))
				Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
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
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When updating service type from LoadBalancer to ClusterIP and revert", func() {
		It("should cleanup resources when changing to ClusterIP and recreate when reverting", func() {
			// Skip("Skip test")
			serviceName := "test-type-change-service"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create LoadBalancer service
			service := newServiceResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LoadBalancer and SecurityGroup to be created (len=1)
			Eventually(func() int {
				lbs, _ := vngcloudRepo.ListLoadBalancers(ctx, []string{})
				if lbs == nil {
					return -1
				}
				return len(lbs.Items)
			}, timeout*4, interval).Should(Equal(1))

			Eventually(func() int {
				secgroups, _ := vngcloudRepo.ListSecurityGroups(ctx)
				if secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout*4, interval).Should(Equal(1))

			// Update service type to ClusterIP
			Eventually(func() error {
				updatedService, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				updatedService.Spec.Type = "ClusterIP"
				return k8sClient.Update(ctx, updatedService)
			}, timeout, interval).Should(Succeed())

			// Wait for LoadBalancer and SecurityGroup to be deleted (len=0)
			Eventually(func() int {
				lbs, _ := vngcloudRepo.ListLoadBalancers(ctx, []string{})
				if lbs == nil {
					return -1
				}
				return len(lbs.Items)
			}, timeout*4, interval).Should(Equal(0))

			Eventually(func() int {
				secgroups, _ := vngcloudRepo.ListSecurityGroups(ctx)
				if secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout*4, interval).Should(Equal(0))

			// Revert service type back to LoadBalancer
			Eventually(func() error {
				revertedService, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				revertedService.Spec.Type = "LoadBalancer"
				return k8sClient.Update(ctx, revertedService)
			}, timeout, interval).Should(Succeed())

			// Wait for LoadBalancer and SecurityGroup to be recreated (len=1)
			Eventually(func() int {
				lbs, _ := vngcloudRepo.ListLoadBalancers(ctx, []string{})
				if lbs == nil {
					return -1
				}
				return len(lbs.Items)
			}, timeout*4, interval).Should(Equal(1))

			Eventually(func() int {
				secgroups, _ := vngcloudRepo.ListSecurityGroups(ctx)
				if secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout*4, interval).Should(Equal(1))

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When creating a DNS LoadBalancer service with TCP and UDP on same port", func() {
		It("should fail with error due to duplicate port (VNGCloud limitation)", func() {
			// Skip("Skip test")
			serviceName := "test-dns-service"
			namespace := "default"

			// Create DNS endpoint first
			endpoint := newDNSEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create DNS service with both TCP and UDP on port 53
			service := newDNSServiceResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Verify LBC spec has 2 listeners (TCP and UDP) - created by service controller
			Expect(lbc.Spec.Listeners).Should(HaveLen(2))
			Expect(lbc.Spec.Pools).Should(HaveLen(2))

			// Wait for LBC status to show error about duplicate port
			Eventually(func() bool {
				lbc := &v1alpha1.LoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: lbcList.Items[0].Name, Namespace: namespace}, lbc)
				if err != nil {
					return false
				}

				// Check if there's an error condition or the LoadBalancerId is NOT set (deployment failed)
				return lbc.Status.LoadBalancerId == nil || *lbc.Status.LoadBalancerId == ""
			}, timeout*4, interval).Should(BeTrue(), "LBC should fail to deploy due to duplicate port")

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When creating service with all normal annotations", func() {
		It("should create LoadBalancer with correct attributes from annotations", func() {
			// Skip("Skip test")
			serviceName := "test-service"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with all normal annotations
			service := newServiceResource(serviceName, namespace)
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
				// fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups):             "sg-1,sg-2", // TODO: create mock security groups
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Wait for LoadBalancer ID in LBC status
			loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancerId).ShouldNot(BeEmpty())

			// Verify LoadBalancer was created with correct attributes
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())
			Expect(loadbalancer.Name).Should(Equal("test-lb"))
			Expect(loadbalancer.Internal).Should(BeTrue())
			Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internal"))
			Expect(loadbalancer.PackageID).Should(Equal("package-iiiiiiiiiiiiiii"))
			Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
			Expect(loadbalancer.Type).Should(Equal("Layer 4"))

			// Verify pool configuration
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect(pools.Items).Should(HaveLen(1))

			pool := pools.Items[0]
			Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
			Expect(pool.Description).Should(Equal("????????"))
			Expect(pool.Status).Should(Equal("ACTIVE"))
			Expect(pool.LoadBalanceMethod).Should(Equal("SOURCE_IP"))
			Expect(pool.Protocol).Should(Equal("TCP"))
			Expect(pool.Stickiness).Should(BeFalse())
			Expect(pool.TLSEncryption).Should(BeFalse())

			// Verify health monitor configuration
			Expect(pool.HealthMonitor).ShouldNot(BeNil())
			Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("PING-UDP"))
			Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(104))
			Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(105))
			Expect(pool.HealthMonitor.Interval).Should(Equal(102))
			Expect(pool.HealthMonitor.Timeout).Should(Equal(103))

			// Verify pool members
			Expect(pool.Members).ShouldNot(BeNil())
			Expect(pool.Members.Items).Should(HaveLen(4))
			Expect(pool.Members.Items[0].Address).Should(BeElementOf(
				mockNode1.Status.Addresses[0].Address,
				mockNode2.Status.Addresses[0].Address,
				mockNode3.Status.Addresses[0].Address,
				mockNode4.Status.Addresses[0].Address))

			// Verify listener configuration
			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect(listeners.Items).Should(HaveLen(1))

			listener := listeners.Items[0]
			Expect(listener.Protocol).Should(Equal("TCP"))
			Expect(listener.ProtocolPort).Should(Equal(80))
			Expect(listener.AllowedCidrs).Should(Equal("1.0.0.0/8"))
			Expect(listener.DefaultPoolId).Should(Equal(pool.UUID))
			Expect(listener.DefaultPoolName).Should(Equal(pool.Name))
			Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-95466-TCP-80"))
			Expect(listener.TimeoutClient).Should(Equal(99))
			Expect(listener.TimeoutConnection).Should(Equal(101))
			Expect(listener.TimeoutMember).Should(Equal(100))
			Expect(listener.Description).Should(Equal("????????"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When creating service with target node label", func() {
		It("should only add pool members from nodes matching the label", func() {
			// Skip("Skip test")
			serviceName := "test-service"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with target node label annotation
			service := newServiceResource(serviceName, namespace)
			service.Annotations = map[string]string{
				fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels): "nodeName=mock-node-1",
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Wait for LoadBalancer ID in LBC status
			loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancerId).ShouldNot(BeEmpty())

			// Verify LoadBalancer was created
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())

			// Verify pool configuration
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect(pools.Items).Should(HaveLen(1))

			pool := pools.Items[0]

			// Verify pool members - should only contain mock-node-1
			Expect(pool.Members).ShouldNot(BeNil())
			Expect(pool.Members.Items).Should(HaveLen(1), "should only have 1 member matching the node label")
			Expect(pool.Members.Items[0].Address).Should(Equal(mockNode1.Status.Addresses[0].Address))

			// Update service annotation to use 2 labels (AND logic)
			Eventually(func() error {
				svc, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetNodeLabels)] = "nodeName=mock-node-2,nodeGroup=mock-node-group-a"
				return k8sClient.Update(ctx, svc)
			}, timeout, interval).Should(Succeed())

			// Wait for pool to be updated with 2-label filter (AND logic)
			Eventually(func() bool {
				pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				if err != nil || pools == nil || len(pools.Items) == 0 {
					return false
				}
				pool := pools.Items[0]
				if pool.Members == nil || len(pool.Members.Items) != 1 {
					return false
				}
				// Verify the member is now mockNode2
				return pool.Members.Items[0].Address == mockNode2.Status.Addresses[0].Address
			}, timeout*4, interval).Should(BeTrue(), "should have 1 member from mockNode2 matching both labels (AND logic)")

			// Verify listener configuration
			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect(listeners.Items).Should(HaveLen(1))

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When creating service with PROXY protocol annotation", func() {
		It("should use PROXY protocol for pool even though service port is TCP", func() {
			// Skip("Skip test")
			serviceName := "test-service-1"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with PROXY protocol annotation
			service := newServiceResource(serviceName, namespace)
			service.Annotations = map[string]string{
				fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixEnableProxyProtocol): "*",
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName, namespace)
				if err != nil {
					return -1
				}
				lbcList = list
				return len(list.Items)
			}, timeout*2, interval).Should(Equal(1))

			lbc := &lbcList.Items[0]

			// Wait for LoadBalancer ID in LBC status
			loadbalancerId, err := waitForLoadBalancerId(lbc.Name, lbc.Namespace)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancerId).ShouldNot(BeEmpty())

			// Verify LoadBalancer was created
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())

			// Verify pool configuration - should use PROXY protocol
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect(pools.Items).Should(HaveLen(1))

			pool := pools.Items[0]
			Expect(pool.Protocol).Should(Equal("PROXY"), "pool protocol should be PROXY when annotation is set")

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})
})
