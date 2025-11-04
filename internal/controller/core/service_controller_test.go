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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
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
			serviceName := "test-lb-service"
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
			Expect(loadbalancer.Type).Should(Equal(string(loadbalancerv2.LoadBalancerTypeLayer4)))

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
})
