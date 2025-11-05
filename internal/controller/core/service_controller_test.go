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
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
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

	Context("When updating service port", func() {
		It("should delete old listener and pool, and create new ones with updated port", func() {
			// Skip("Skip test")
			serviceName := "test-service-1"
			namespace := "default"

			// Create endpoint first
			endpoint := newEndpointResource(serviceName, namespace)
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with port 80
			service := newServiceResource(serviceName, namespace)
			service.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(80)},
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

			// Verify initial pool configuration with port 80
			pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(pools).ShouldNot(BeNil())
			Expect(pools.Items).Should(HaveLen(1))

			pool := pools.Items[0]
			Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
			Expect(pool.Protocol).Should(Equal("TCP"))

			Expect(pool.Members).ShouldNot(BeNil())
			Expect(pool.Members.Items).Should(HaveLen(4))

			// Verify initial listener configuration with port 80
			listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect(listeners.Items).Should(HaveLen(1))

			listener := listeners.Items[0]
			Expect(listener.Protocol).Should(Equal("TCP"))
			Expect(listener.ProtocolPort).Should(Equal(80))
			Expect(listener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))

			// Update service port from 80 to 81
			Eventually(func() error {
				updatedService, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				updatedService.Spec.Ports = []corev1.ServicePort{
					{Name: "http", Port: 81, TargetPort: intstr.FromInt(80)},
				}
				return k8sClient.Update(ctx, updatedService)
			}, timeout, interval).Should(Succeed())

			// Wait for pool to be updated - should have new pool with port 81
			Eventually(func() bool {
				pools, err = vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				if err != nil || pools == nil {
					return false
				}
				return len(pools.Items) == 1 && pools.Items[0].Name == "vks-k8s-000000-default-test-serv-75a17-TCP-81"
			}, timeout*4, interval).Should(BeTrue(), "pool should be updated to port 81")

			// Verify updated pool configuration with port 81
			updatedPool := pools.Items[0]
			Expect(updatedPool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-81"))

			Expect(updatedPool.Members).ShouldNot(BeNil())
			Expect(updatedPool.Members.Items).Should(HaveLen(4))
			Expect(updatedPool.Members.Items[0].Address).Should(BeElementOf(
				mockNode1.Status.Addresses[0].Address,
				mockNode2.Status.Addresses[0].Address,
				mockNode3.Status.Addresses[0].Address,
				mockNode4.Status.Addresses[0].Address))

			// Verify updated listener configuration with port 81
			listeners, err = vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(listeners).ShouldNot(BeNil())
			Expect(listeners.Items).Should(HaveLen(1))

			updatedListener := listeners.Items[0]
			Expect(updatedListener.Protocol).Should(Equal("TCP"))
			Expect(updatedListener.ProtocolPort).Should(Equal(81))
			Expect(updatedListener.DefaultPoolId).Should(Equal(updatedPool.UUID))
			Expect(updatedListener.DefaultPoolName).Should(Equal(updatedPool.Name))
			Expect(updatedListener.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-81"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When managing security groups and tags with Cilium Native CNI", func() {

		It("should manage security groups and tags with updates", func() {
			// Skip("Skip test")

			// Configure CNI detector for Cilium Native
			cniDetector.ExpectedCalls = nil
			cniDetector.Calls = nil
			cniDetector.EXPECT().DetectCNIType(mock.Anything).Return(utils.CiliumNativeRouting, nil)

			// Create test security groups
			bigbangSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "bigbang", "the best security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(bigbangSec).ShouldNot(BeNil())
			blackpinkSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "blackpink", "the great security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blackpinkSec).ShouldNot(BeNil())

			defer func() {
				// Clean up security groups
				Eventually(func() bool {
					err = vngcloudRepo.DeleteSecurityGroup(ctx, bigbangSec.Id)
					return err == nil
				}, timeout, interval).Should((BeTrue()), "should delete bigbang security group (wait to detach if in use)")
				Eventually(func() bool {
					err = vngcloudRepo.DeleteSecurityGroup(ctx, blackpinkSec.Id)
					return err == nil
				}, timeout, interval).Should((BeTrue()), "should delete blackpink security group (wait to detach if in use)")
			}()

			serviceName := "test-service-gogsf"
			namespace := "default"

			// Create custom endpoint with two subsets, each with different port mappings
			endpoint := newEndpointResource(serviceName, namespace)
			endpoint.Subsets = []corev1.EndpointSubset{
				// First subset - Deployment pods with ports 80 and 443
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
				// Second subset - Different pods with ports 8080 and 6443
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
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with named target port "http" and specific NodePort
			service := newServiceResource(serviceName, namespace)
			service.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), NodePort: 31000},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LoadBalancer
			var loadbalancerId string
			Eventually(func() bool {
				lbcList, err := getLBCListForService(serviceName, namespace)
				if err != nil || len(lbcList.Items) == 0 {
					return false
				}
				id, err := waitForLoadBalancerId(lbcList.Items[0].Name, lbcList.Items[0].Namespace)
				if err == nil && id != "" {
					loadbalancerId = id
					return true
				}
				return false
			}, timeout*4, interval).Should(BeTrue())

			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())

			// Verify initial state: default VKS tag
			tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(tags.Items).Should(HaveLen(1))
			Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))

			// Verify 3 security groups exist (default + 2 test groups)
			secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
			Eventually(func() int {
				secgroups, err = vngcloudRepo.ListSecurityGroups(ctx)
				if err != nil || secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout, interval).Should(Equal(3), "should have 3 security groups: default + bigbang + blackpink")

			// Find default security group and verify rules
			var defaultSecgroupID string
			for _, sg := range secgroups.Items {
				if sg.Name == "vks-k8s-000000-default-test-servi-4d0e7" {
					defaultSecgroupID = sg.Id
					break
				}
			}
			Expect(defaultSecgroupID).ShouldNot(BeEmpty(), "should find default security group")

			// Verify security group rules for Cilium Native (should have pod port rules + nodeport + egress)
			// For Cilium Native: 1 nodeport + 6 pod ports (2 ports x 3 subnets) + 2 egress = 9 rules
			Eventually(func() int {
				rules, err := vngcloudRepo.ListSecurityGroupRules(ctx, defaultSecgroupID)
				if err != nil || rules == nil {
					return -1
				}
				return len(rules.Items)
			}, timeout, interval).Should(Equal(9), "Cilium should have 9 rules: 1 nodeport + 6 pod ports + 2 egress")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, defaultSecgroupID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have default security group attached")

			// Update tags and security groups
			Eventually(func() error {
				svc, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				if svc.Annotations == nil {
					svc.Annotations = make(map[string]string)
				}
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag1=value1,tag2=value2"
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = bigbangSec.Id
				return k8sClient.Update(ctx, svc)
			}, timeout, interval).Should(Succeed())

			// Verify tags updated (VKS + 2 custom)
			Eventually(func() int {
				tags, err = vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
				Expect(err).ShouldNot(HaveOccurred())
				return len(tags.Items)
			}, timeout, interval).Should(Equal(3), "should have 3 tags after update")

			// Verify default secgroup deleted, only 2 remain
			Eventually(func() int {
				secgroups, err = vngcloudRepo.ListSecurityGroups(ctx)
				if err != nil || secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout, interval).Should(Equal(2), "should have 2 security groups: bigbang + blackpink after default removed")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have bigbang security group attached")

			// Update tags and switch to blackpink secgroup
			Eventually(func() error {
				svc, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag2=value22,tag3=value3"
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = blackpinkSec.Id
				return k8sClient.Update(ctx, svc)
			}, timeout, interval).Should(Succeed())

			// Verify tags updated
			Eventually(func() bool {
				tags, err = vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
				if err != nil || tags == nil || len(tags.Items) != 3 {
					return false
				}
				expectKeys := []string{consts.VKS_TAG_KEY, "tag2", "tag3"}
				for _, tag := range tags.Items {
					if !slices.Contains(expectKeys, tag.Key) {
						return false
					}
				}
				return true
			}, timeout, interval).Should(Equal(true), "should have updated tags after second update")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(0), "all 4 nodes should have bigbang security group attached")
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have blackpink security group attached")

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When managing security groups and tags with Calico Overlay CNI", func() {

		It("should manage security groups and tags with updates", func() {
			// Skip("Skip test")

			// Configure CNI detector for Calico
			cniDetector.ExpectedCalls = nil
			cniDetector.Calls = nil
			cniDetector.EXPECT().DetectCNIType(mock.Anything).Return(utils.CalicoOverlay, nil)

			// Create test security groups
			bigbangSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "bigbang", "the best security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(bigbangSec).ShouldNot(BeNil())
			blackpinkSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "blackpink", "the great security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blackpinkSec).ShouldNot(BeNil())

			defer func() {
				// Clean up security groups
				Eventually(func() bool {
					err = vngcloudRepo.DeleteSecurityGroup(ctx, bigbangSec.Id)
					return err == nil
				}, timeout, interval).Should((BeTrue()), "should delete bigbang security group (wait to detach if in use)")
				Eventually(func() bool {
					err = vngcloudRepo.DeleteSecurityGroup(ctx, blackpinkSec.Id)
					return err == nil
				}, timeout, interval).Should((BeTrue()), "should delete blackpink security group (wait to detach if in use)")
			}()

			serviceName := "test-service-gogsf"
			namespace := "default"

			// Create custom endpoint with two subsets, each with different port mappings
			endpoint := newEndpointResource(serviceName, namespace)
			endpoint.Subsets = []corev1.EndpointSubset{
				// First subset - Deployment pods with ports 80 and 443
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
				// Second subset - Different pods with ports 8080 and 6443
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
			Expect(k8sClient.Create(ctx, endpoint)).Should(Succeed())

			// Create service with named target port "http" and specific NodePort
			service := newServiceResource(serviceName, namespace)
			service.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP, NodePort: 30000},
			}
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// Wait for LoadBalancer
			var loadbalancerId string
			Eventually(func() bool {
				lbcList, err := getLBCListForService(serviceName, namespace)
				if err != nil || len(lbcList.Items) == 0 {
					return false
				}
				id, err := waitForLoadBalancerId(lbcList.Items[0].Name, lbcList.Items[0].Namespace)
				if err == nil && id != "" {
					loadbalancerId = id
					return true
				}
				return false
			}, timeout*4, interval).Should(BeTrue())

			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())

			// Verify initial state: default VKS tag
			tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(tags.Items).Should(HaveLen(1))
			Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))

			// Verify 3 security groups exist (default + 2 test groups)
			secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
			Eventually(func() int {
				secgroups, err = vngcloudRepo.ListSecurityGroups(ctx)
				if err != nil || secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout, interval).Should(Equal(3), "should have 3 security groups: default + bigbang + blackpink")

			// Find default security group and verify rules
			var defaultSecgroupID string
			for _, sg := range secgroups.Items {
				if sg.Name == "vks-k8s-000000-default-test-servi-4d0e7" {
					defaultSecgroupID = sg.Id
					break
				}
			}
			Expect(defaultSecgroupID).ShouldNot(BeEmpty(), "should find default security group")

			// Verify Calico only has NodePort rule (not pod port rules)
			Eventually(func() int {
				rules, err := vngcloudRepo.ListSecurityGroupRules(ctx, defaultSecgroupID)
				if err != nil || rules == nil {
					return -1
				}
				return len(rules.Items)
			}, timeout, interval).Should(Equal(3), "Calico should only have 1 nodeport + 2 egress rules")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, defaultSecgroupID)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have default security group attached")

			// Update tags and security groups
			Eventually(func() error {
				svc, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				if svc.Annotations == nil {
					svc.Annotations = make(map[string]string)
				}
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag1=value1,tag2=value2"
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = bigbangSec.Id
				return k8sClient.Update(ctx, svc)
			}, timeout, interval).Should(Succeed())

			// Verify tags updated (VKS + 2 custom)
			Eventually(func() int {
				tags, err = vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
				Expect(err).ShouldNot(HaveOccurred())
				return len(tags.Items)
			}, timeout, interval).Should(Equal(3), "should have 3 tags after update")

			// Verify default secgroup deleted, only 2 remain
			Eventually(func() int {
				secgroups, err = vngcloudRepo.ListSecurityGroups(ctx)
				if err != nil || secgroups == nil {
					return -1
				}
				return len(secgroups.Items)
			}, timeout, interval).Should(Equal(2), "should have 2 security groups: bigbang + blackpink after default removed")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have bigbang security group attached")

			// Update tags and switch to blackpink secgroup
			Eventually(func() error {
				svc, err := getServiceResource(serviceName, namespace)
				if err != nil {
					return err
				}
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTags)] = "tag2=value22,tag3=value3"
				svc.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixSecurityGroups)] = blackpinkSec.Id
				return k8sClient.Update(ctx, svc)
			}, timeout, interval).Should(Succeed())

			// Verify tags updated
			Eventually(func() bool {
				tags, err = vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
				if err != nil || tags == nil || len(tags.Items) != 3 {
					return false
				}
				expectKeys := []string{consts.VKS_TAG_KEY, "tag2", "tag3"}
				for _, tag := range tags.Items {
					if !slices.Contains(expectKeys, tag.Key) {
						return false
					}
				}
				return true
			}, timeout, interval).Should(Equal(true), "should have updated tags after second update")

			// Verify servers have the security group attached
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(0), "all 4 nodes should have bigbang security group attached")
			Eventually(func() int {
				server, err := vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(server).ShouldNot(BeNil())
				return len(server.Items)
			}, timeout, interval).Should(Equal(4), "all 4 nodes should have blackpink security group attached")

			// Cleanup
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, endpoint)).Should(Succeed())
		})
	})

	Context("When create 3 services using same LB", func() {
		It("should work well, delete should delete all", func() {
			// Skip("Skip test")
			serviceName1 := "test-service-port-80"
			serviceName2 := "test-service-port-81"
			serviceName3 := "test-service-port-82"
			namespace := "default"
			var lbID string

			// Create first service with port 80
			service1 := newServiceResource(serviceName1, namespace)
			service1.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(80), Protocol: corev1.ProtocolTCP, NodePort: 30000},
			}
			Expect(k8sClient.Create(ctx, service1)).Should(Succeed())

			// Wait for LBC to be created
			var lbcList *v1alpha1.LoadBalancerConfigList
			Eventually(func() int {
				list, err := getLBCListForService(serviceName1, namespace)
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
			lbID = loadbalancerId

			// Get load balancer
			loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(loadbalancer).ShouldNot(BeNil())

			// Check pool - should have 1
			Eventually(func() int {
				pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				if err != nil || pools == nil {
					return -1
				}
				return len(pools.Items)
			}, timeout*2, interval).Should(Equal(1))

			// Check listener - should have 1
			Eventually(func() int {
				listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
				if err != nil || listeners == nil {
					return -1
				}
				return len(listeners.Items)
			}, timeout*2, interval).Should(Equal(1))

			// Create second service with port 81 and same LB ID annotation
			service2 := newServiceResource(serviceName2, namespace)
			service2.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 81, TargetPort: intstr.FromInt(81), Protocol: corev1.ProtocolTCP, NodePort: 30001},
			}
			if service2.Annotations == nil {
				service2.Annotations = map[string]string{}
			}
			service2.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)] = lbID
			Expect(k8sClient.Create(ctx, service2)).Should(Succeed())

			// Check pool - should have 2
			Eventually(func() int {
				pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				if err != nil || pools == nil {
					return -1
				}
				return len(pools.Items)
			}, timeout*4, interval).Should(Equal(2))

			// Check listener - should have 2
			Eventually(func() int {
				listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
				if err != nil || listeners == nil {
					return -1
				}
				return len(listeners.Items)
			}, timeout*4, interval).Should(Equal(2))

			// Create third service with port 82 and same LB ID annotation
			service3 := newServiceResource(serviceName3, namespace)
			service3.Spec.Ports = []corev1.ServicePort{
				{Name: "http", Port: 82, TargetPort: intstr.FromInt(82), Protocol: corev1.ProtocolTCP, NodePort: 30002},
			}
			if service3.Annotations == nil {
				service3.Annotations = map[string]string{}
			}
			service3.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)] = lbID
			Expect(k8sClient.Create(ctx, service3)).Should(Succeed())

			// Check pool - should have 3
			Eventually(func() int {
				pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
				if err != nil || pools == nil {
					return -1
				}
				return len(pools.Items)
			}, timeout*4, interval).Should(Equal(3))

			// Check listener - should have 3
			Eventually(func() int {
				listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
				if err != nil || listeners == nil {
					return -1
				}
				return len(listeners.Items)
			}, timeout*4, interval).Should(Equal(3))

			// Delete all services
			Expect(k8sClient.Delete(ctx, service1)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service2)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, service3)).Should(Succeed())

			// Wait for load balancer to be deleted
			Eventually(func() int {
				listLB, err := vngcloudRepo.ListLoadBalancers(ctx, nil)
				if err != nil {
					return -1
				}
				return len(listLB.Items)
			}, timeout*4, interval).Should(Equal(0))
		})
	})
})
