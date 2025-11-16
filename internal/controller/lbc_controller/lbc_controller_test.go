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

package lbc_controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

var _ = Describe("LoadBalancerConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		vngcloudloadbalancerconfig := &v1alpha1.LoadBalancerConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind LoadBalancerConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, vngcloudloadbalancerconfig)
			if err != nil && apierrors.IsNotFound(err) {
				resource := &v1alpha1.LoadBalancerConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: v1alpha1.LoadBalancerConfigSpec{
						Type:             "Layer 4",
						LoadBalancerName: "TODO",
						SubnetId:         "TODO",
						ZoneId:           "TODO",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &v1alpha1.LoadBalancerConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance LoadBalancerConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			// Skip("Skip test")
			By("Reconciling the created resource")
			controllerReconciler := &LoadBalancerConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When create service with specific annotation", func() {
		It("created load balancer shoud have specific attribute", func() {
			// Skip("Skip test")
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId)) // TODO: fix me
						// Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID())) // TODO: fix me
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

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
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
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
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

						// check pool
						pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
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
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
						}

						// check listener
						listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

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
							Expect((pool.Members.Items)).Should(HaveLen(1)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address))
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

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
							Expect((pool.Members.Items)).Should(HaveLen(1)) // number of member in pool = number of node OR number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address))
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

						// check pool
						pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-PRO-80"))
							Expect(pool.Description).Should(Equal("????????"))
							Expect(pool.Status).Should(Equal("ACTIVE"))
							Expect(pool.LoadBalanceMethod).Should(Equal(mockConfig.LoadBalancerOpts.DefaultPoolAlgorithm))
							Expect(pool.Protocol).Should(Equal("PROXY"))
							Expect(pool.Stickiness).Should(BeFalse())
							Expect(pool.TLSEncryption).Should(BeFalse())

							Expect(pool.HealthMonitor).ShouldNot(BeNil())
							Expect(pool.HealthMonitor.HealthCheckProtocol).Should(Equal("TCP"))
							Expect(pool.HealthMonitor.HealthyThreshold).Should(Equal(mockConfig.LoadBalancerOpts.DefaultHealthyThreshold))
							Expect(pool.HealthMonitor.UnhealthyThreshold).Should(Equal(mockConfig.LoadBalancerOpts.DefaultUnhealthyThreshold))
							Expect(pool.HealthMonitor.Interval).Should(Equal(mockConfig.LoadBalancerOpts.DefaultInterval))
							Expect(pool.HealthMonitor.Timeout).Should(Equal(mockConfig.LoadBalancerOpts.DefaultTimeout))

							Expect(pool.Members).ShouldNot(BeNil())
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

						// check pool
						pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
							Expect(pool.Members.Items[0].Address).Should(BeElementOf(
								vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
								vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
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
								// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
								Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 4"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

								// check pool
								pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
								Eventually(func() bool {
									pools, err = vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
									Expect(err).ShouldNot(HaveOccurred())
									Expect(pools).ShouldNot(BeNil())
									return len(pools.Items) == 1 && pools.Items[0].Name == "vks-k8s-000000-default-test-serv-75a17-TCP-81"
								}, time.Second*10, interval).Should(Equal(true))
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-81"))
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
									Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
									Expect(pool.Members.Items[0].Address).Should(BeElementOf(
										vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
										vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
										vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
										vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
								}

								// check listener
								listeners, err := vngcloudRepo.ListListenerOfLB(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(listeners).ShouldNot(BeNil())
								Expect((listeners.Items)).Should(HaveLen(1)) // number of listener
								for _, listener := range listeners.Items {
									Expect(listener.Protocol).Should(Equal("TCP"))
									Expect(listener.ProtocolPort).Should(Equal(81))
									Expect(listener.AllowedCidrs).Should(Equal(mockConfig.LoadBalancerOpts.DefaultAllowedCidrs))
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
									{IP: "100.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
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
						// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
						Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
						Expect(loadbalancer.Type).Should(Equal("Layer 4"))
						// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

						// check pool
						pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(pools).ShouldNot(BeNil())
						Expect((pools.Items)).Should(HaveLen(1)) // number of pool
						for _, pool := range pools.Items {
							Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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
							Expect((pool.Members.Items)).Should(HaveLen(4)) // number of member in pool = number of node or number of endpoint
							for _, member := range pool.Members.Items {
								Expect(member.ProtocolPort).Should(Equal(31000))
								Expect(member.MonitorPort).Should(Equal(31000))
								Expect(member.Address).Should(BeElementOf(
									vngcloud_mocks.MockNode1.Status.Addresses[0].Address,
									vngcloud_mocks.MockNode2.Status.Addresses[0].Address,
									vngcloud_mocks.MockNode3.Status.Addresses[0].Address,
									vngcloud_mocks.MockNode4.Status.Addresses[0].Address))
							}
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
								object.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixTargetType)] = string(domain.TargetTypeIP)
								return []client.Object{&object}
							},
							expect: func(loadbalancer *entity.LoadBalancer) {
								// wait until reconcile done
								time.Sleep(timeWaitRecocile)

								Expect(loadbalancer).ShouldNot(BeNil())
								Expect(loadbalancer.Name).Should(Equal("vks-k8s-000000-default-test-servi-75a17"))
								Expect(loadbalancer.Internal).Should(BeFalse())
								Expect(loadbalancer.LoadBalancerSchema).Should(Equal("Internet"))
								// Expect(loadbalancer.PackageID).Should(Equal(mockConfig.LoadBalancerOpts.DefaultL4PackageId))
								Expect(loadbalancer.SubnetID).Should(Equal(vngcloudRepo.GetDefaultSubnetID()))
								Expect(loadbalancer.Type).Should(Equal("Layer 4"))
								// Expect(loadbalancer.PrivateSubnetCidr).Should(Equal(vngcloudRepo.GetSubnetCIDR()))

								// check pool
								pools, err := vngcloudRepo.ListPool(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(pools).ShouldNot(BeNil())
								Expect((pools.Items)).Should(HaveLen(1)) // number of pool
								for _, pool := range pools.Items {
									Expect(pool.Name).Should(Equal("vks-k8s-000000-default-test-serv-75a17-TCP-80"))
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

				// expect create vlbc with label: belong-to-service=test-service. List vlbc to check
				vlbcList := &v1alpha1.LoadBalancerConfigList{}
				Eventually(func() bool {
					err := k8sClient.List(ctx, vlbcList, client.InNamespace("default"), client.MatchingLabels{
						consts.LabelOwnerResourceName: service.Name,
					})
					if err != nil {
						return false
					}
					return len(vlbcList.Items) == 1
				}, time.Second*10, interval).Should(BeTrue())

				vlbc := &v1alpha1.LoadBalancerConfig{}
				vlbc = &vlbcList.Items[0]
				Expect(vlbc.Spec.Type).Should(Equal(loadbalancerv2.LoadBalancerTypeLayer4))
				// logrus.Infof("VLBC created:%+v", vlbc)

				// get load balancer id from vlbc status
				loadbalancerId := ""
				Eventually(func() bool {
					getVLBC := &v1alpha1.LoadBalancerConfig{}
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: vlbc.Name, Namespace: vlbc.Namespace}, getVLBC)).Should(Succeed())
					if getVLBC.Status.LoadBalancerId != nil {
						loadbalancerId = *getVLBC.Status.LoadBalancerId
						return loadbalancerId != ""
					}
					return false
				}, time.Second*10, interval).Should(BeTrue())

				// expect load balancer attribute in the mock provider
				loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
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
				_, err = vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerId)
				Expect(err).Should(HaveOccurred())
				printEndTest()
			}
		})
	})

	Context("When update tags and secgroups annotations", func() {
		It("load balancer and server should do expect behavior", func() {
			// Skip("Skip test")
			// add 2 foo security group
			bigbangSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "bigbang", "the best security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(bigbangSec).ShouldNot(BeNil())
			blackpinkSec, err := vngcloudRepo.CreateSecurityGroup(ctx, "blackpink", "the great security group")
			Expect(err).ShouldNot(HaveOccurred())
			Expect(blackpinkSec).ShouldNot(BeNil())

			// delete when finish
			defer func() {
				Expect(vngcloudRepo.DeleteSecurityGroup(ctx, bigbangSec.Id)).Should(Succeed())
				Expect(vngcloudRepo.DeleteSecurityGroup(ctx, blackpinkSec.Id)).Should(Succeed())
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
						// Clear existing expectations and set new one
						cniDetector.ExpectedCalls = nil
						cniDetector.Calls = nil
						cniDetector.EXPECT().DetectCNIType(mock.Anything).Return(utils.CiliumNativeRouting, nil)
					},
					name: "create with default annotations of cilium native routing",
					generateDepends: func() []client.Object {
						endpoint := newEndpointResource("test-service-gogsf", "default")
						endpoint.Subsets = []corev1.EndpointSubset{
							// endpointSubset is for Deployment,... which is in use by service
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "100.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
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
						tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(tags).ShouldNot(BeNil())
						Eventually(func() bool {
							tags, err = vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(tags).ShouldNot(BeNil())
							return len(tags.Items) == 1 && tags.Items[0].Key == consts.VKS_TAG_KEY && tags.Items[0].Value == mockConfig.Cluster.ClusterID
						}, time.Second*10, interval).Should(Equal(true))

						// check secgroups
						secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
						Eventually(func() bool {
							secgroups, err = vngcloudRepo.ListSecurityGroups(ctx)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(secgroups).ShouldNot(BeNil())
							return len(secgroups.Items) == 3
						}, time.Second*10, interval).Should(Equal(true))
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
						rules, err := vngcloudRepo.ListSecurityGroupRules(ctx, secgroupID)
						Eventually(func() []*entity.SecgroupRule {
							rules, err = vngcloudRepo.ListSecurityGroupRules(ctx, secgroupID)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(rules).ShouldNot(BeNil())
							return rules.Items
						}, time.Second*10, interval).Should(HaveLen(9)) // 1 nodeport + 3 x 2 (2 pod ports x 3 subnets (4 nodes in 3 subnets)) + 2 engress default allow all
						expectPortRangeMax := []int{80, 80, 80, 8080, 8080, 8080, 31000, 65535, 65535}
						expectPortRangeMin := []int{80, 80, 80, 8080, 8080, 8080, 31000, 0, 0}
						expectCIDRs := []string{
							vngcloud_mocks.MockSubnetCIDR, vngcloud_mocks.MockSubnetCIDR, vngcloud_mocks.MockSubnetCIDR,
							vngcloud_mocks.MockSubnetCIDR_1b_1, vngcloud_mocks.MockSubnetCIDR_1b_1,
							vngcloud_mocks.MockSubnetCIDR_1b_2, vngcloud_mocks.MockSubnetCIDR_1b_2,
							"::/0", "0.0.0.0/0",
						}
						for _, rule := range rules.Items {
							Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
							expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
							Expect(rule.PortRangeMin).Should(BeElementOf(expectPortRangeMin))
							expectPortRangeMin = removeFisrt(expectPortRangeMin, rule.PortRangeMin)
							Expect(rule.Direction).Should(BeElementOf([]string{"ingress", "egress"}))
							Expect(rule.EtherType).Should(BeElementOf([]string{"IPv4", "IPv6"}))
							Expect(rule.Protocol).Should(BeElementOf([]string{"tcp", "any"}))
							Expect(rule.RemoteIPPrefix).Should(BeElementOf(expectCIDRs))
							expectCIDRs = removeFisrt(expectCIDRs, rule.RemoteIPPrefix)
						}

						// check server have secgroup
						server, err := vngcloudRepo.ListServerBySecgroupID(ctx, secgroupID)
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
											{IP: "100.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
											{IP: "100.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
										},
										NotReadyAddresses: []corev1.EndpointAddress{
											{IP: "100.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
											{IP: "100.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
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
								tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(tags).ShouldNot(BeNil())
								Expect((tags.Items)).Should(HaveLen(1))
								Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))
								Expect(tags.Items[0].Value).Should(Equal(mockConfig.Cluster.ClusterID))

								// check secgroups
								secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
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
								rules, err := vngcloudRepo.ListSecurityGroupRules(ctx, secgroupID)
								Eventually(func() []*entity.SecgroupRule {
									rules, err = vngcloudRepo.ListSecurityGroupRules(ctx, secgroupID)
									Expect(err).ShouldNot(HaveOccurred())
									Expect(rules).ShouldNot(BeNil())
									return rules.Items
								}, time.Second*10, interval).Should(HaveLen(6)) // 1 nodeport + 3 x 1 (1 pod port x 3 subnets (4 nodes in 3 subnets)) + 2 engress default allow all
								expectPortRangeMax := []int{80, 80, 80, 31000, 65535, 65535}
								expectCIDRs := []string{
									vngcloud_mocks.MockSubnetCIDR, vngcloud_mocks.MockSubnetCIDR,
									vngcloud_mocks.MockSubnetCIDR_1b_1, vngcloud_mocks.MockSubnetCIDR_1b_2,
									"::/0", "0.0.0.0/0",
								}
								for _, rule := range rules.Items {
									Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
									expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
									// Expect(rule.PortRangeMin).Should(Equal(rule.PortRangeMax))
									Expect(rule.Direction).Should(BeElementOf([]string{"ingress", "egress"}))
									Expect(rule.EtherType).Should(BeElementOf([]string{"IPv4", "IPv6"}))
									Expect(rule.Protocol).Should(BeElementOf([]string{"tcp", "any"}))
									Expect(rule.RemoteIPPrefix).Should(BeElementOf(expectCIDRs))
									expectCIDRs = removeFisrt(expectCIDRs, rule.RemoteIPPrefix)
								}

								// check server have secgroup
								server, err := vngcloudRepo.ListServerBySecgroupID(ctx, secgroupID)
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
								tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
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
								secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should delete default secgroup

								// check server have secgroup
								server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
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
								tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
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
								secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should not delete secgroup

								// check server have secgroup
								server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(BeEmpty())

								server, err = vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
					},
					postTest: func() {
						// ensure no server have secgroup after delete service
						Eventually(func() bool {
							server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(server).ShouldNot(BeNil())
							if len(server.Items) != 0 {
								return false
							}
							server, err = vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(server).ShouldNot(BeNil())
							return len(server.Items) == 0
						}, time.Second*10, interval).Should(Equal(true))
					},
				},
				{
					preTest: func() {
						// Clear existing expectations and set new one
						cniDetector.ExpectedCalls = nil
						cniDetector.Calls = nil
						cniDetector.EXPECT().DetectCNIType(mock.Anything).Return(utils.CalicoOverlay, nil)
					},
					name: "create with default annotations of calico overlay",
					generateDepends: func() []client.Object {
						endpoint := newEndpointResource("test-service-gogsf", "default")
						endpoint.Subsets = []corev1.EndpointSubset{
							// endpointSubset is for Deployment,... which is in use by service
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "100.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "100.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "100.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "mock-pod-4", Kind: "Pod", Namespace: "default"}},
								},
								Ports: []corev1.EndpointPort{
									{Name: "http", Port: 80},
									{Name: "https", Port: 443},
								},
							},
							{
								Addresses: []corev1.EndpointAddress{
									{IP: "200.0.1.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode1.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-1", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.2.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode2.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-2", Kind: "Pod", Namespace: "default"}},
								},
								NotReadyAddresses: []corev1.EndpointAddress{
									{IP: "200.0.3.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode3.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-3", Kind: "Pod", Namespace: "default"}},
									{IP: "200.0.4.0", Hostname: "", NodeName: &vngcloud_mocks.MockNode4.Name, TargetRef: &corev1.ObjectReference{Name: "fake-pod-4", Kind: "Pod", Namespace: "default"}},
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
						tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(tags).ShouldNot(BeNil())
						Expect((tags.Items)).Should(HaveLen(1))
						Expect(tags.Items[0].Key).Should(Equal(consts.VKS_TAG_KEY))
						Expect(tags.Items[0].Value).Should(Equal(mockConfig.Cluster.ClusterID))

						// check secgroups
						secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
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
						rules, err := vngcloudRepo.ListSecurityGroupRules(ctx, secgroupID)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(rules).ShouldNot(BeNil())
						Expect((rules.Items)).Should(HaveLen(3))
						expectPortRangeMax := []int{30000, 65535, 65535} // calico overlay should only have nodeport
						for _, rule := range rules.Items {
							Expect(rule.PortRangeMax).Should(BeElementOf(expectPortRangeMax))
							expectPortRangeMax = removeFisrt(expectPortRangeMax, rule.PortRangeMax)
							// Expect(rule.PortRangeMin).Should(Equal(rule.PortRangeMax))
							Expect(rule.Direction).Should(BeElementOf([]string{"ingress", "egress"}))
							Expect(rule.EtherType).Should(BeElementOf([]string{"IPv4", "IPv6"}))
							Expect(rule.Protocol).Should(BeElementOf([]string{"tcp", "any"}))
							Expect(rule.RemoteIPPrefix).Should(BeElementOf([]string{vngcloud_mocks.MockSubnetCIDR, "0.0.0.0/0", "::/0"}))
						}

						// check server have secgroup
						server, err := vngcloudRepo.ListServerBySecgroupID(ctx, secgroupID)
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
								tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
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
								secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should delete default secgroup

								// check server have secgroup
								server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
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
								tags, err := vngcloudRepo.ListTags(ctx, loadbalancer.UUID)
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
								secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(secgroups).ShouldNot(BeNil())
								Expect((secgroups.Items)).Should(HaveLen(2)) // should not delete secgroup

								// check server have secgroup
								server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(BeEmpty())

								server, err = vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
								Expect(err).ShouldNot(HaveOccurred())
								Expect(server).ShouldNot(BeNil())
								Expect((server.Items)).Should(HaveLen(4))
							},
						},
					},
					postTest: func() {
						// ensure no server have secgroup after delete service
						Eventually(func() bool {
							server, err := vngcloudRepo.ListServerBySecgroupID(ctx, bigbangSec.Id)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(server).ShouldNot(BeNil())
							if len(server.Items) != 0 {
								return false
							}
							server, err = vngcloudRepo.ListServerBySecgroupID(ctx, blackpinkSec.Id)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(server).ShouldNot(BeNil())
							return len(server.Items) == 0
						}, time.Second*10, interval).Should(Equal(true))
					},
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

				// expect create vlbc with label: belong-to-service=test-service. List vlbc to check
				vlbcList := &v1alpha1.LoadBalancerConfigList{}
				Eventually(func() bool {
					err := k8sClient.List(ctx, vlbcList, client.InNamespace("default"), client.MatchingLabels{
						consts.LabelOwnerResourceName: obj.GetName(),
					})
					if err != nil {
						return false
					}
					return len(vlbcList.Items) == 1
				}, time.Second*10, interval).Should(BeTrue())

				vlbc := &v1alpha1.LoadBalancerConfig{}
				vlbc = &vlbcList.Items[0]
				Expect(vlbc.Spec.Type).Should(Equal(loadbalancerv2.LoadBalancerTypeLayer4))
				// logrus.Infof("VLBC created:%+v", vlbc)

				// get load balancer id from vlbc status
				loadbalancerID := ""
				Eventually(func() bool {
					getVLBC := &v1alpha1.LoadBalancerConfig{}
					Expect(k8sClient.Get(ctx, client.ObjectKey{Name: vlbc.Name, Namespace: vlbc.Namespace}, getVLBC)).Should(Succeed())
					if getVLBC.Status.LoadBalancerId != nil {
						loadbalancerID = *getVLBC.Status.LoadBalancerId
						return loadbalancerID != ""
					}
					return false
				}, time.Second*10, interval).Should(BeTrue())

				// // get load balancer id in the annotation
				// loadbalancerID := ""
				// Eventually(func() bool {
				// 	getObj := &corev1.Service{}
				// 	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, getObj)).Should(Succeed())
				// 	loadbalancerID = getObj.Annotations[fmt.Sprintf("%s/%s", consts.SERVICE_ANNOTATION_PREFIX, annotations.SuffixLoadBalancerID)]
				// 	return loadbalancerID != ""
				// }, 10*time.Second, interval).Should(BeTrue())

				// expect load balancer attribute in the mock provider
				loadbalancer, err := vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerID)
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
				_, err = vngcloudRepo.GetLoadBalancerByID(ctx, loadbalancerID)
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
