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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

var _ = Describe("VngcloudLoadBalancerConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		vngcloudloadbalancerconfig := &v1alpha1.VngcloudLoadBalancerConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind VngcloudLoadBalancerConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, vngcloudloadbalancerconfig)
			if err != nil && errors.IsNotFound(err) {
				resource := &v1alpha1.VngcloudLoadBalancerConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
						Type:             "Layer 4",
						LoadBalancerName: "TODO",
						SubnetID:         "TODO",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &v1alpha1.VngcloudLoadBalancerConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance VngcloudLoadBalancerConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &VngcloudLoadBalancerConfigReconciler{
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

			// create default service
			service := newServiceResource("test-service", "default")
			Expect(service).NotTo(BeNil())
			Expect(k8sClient.Create(ctx, service)).Should(Succeed())

			// expect create vlbc with label: belong-to-service=test-service. List vlbc to check
			vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, vlbcList, client.InNamespace("default"), client.MatchingLabels{"belong-to-service": "test-service"})
				if err != nil {
					return false
				}
				return len(vlbcList.Items) == 1
			}, timeout, interval).Should(BeTrue())

			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{}
			vlbc = &vlbcList.Items[0]
			Expect(vlbc.Spec.Type).Should(Equal(loadbalancerv2.LoadBalancerTypeLayer4))
			// Expect(vlbc.Spec.SubnetID).Should(Equal(mockProvider.GetDefaultSubnetID()))
			// Expect(vlbc.Name).Should(Equal("vks-k8s-000000-default-test-servi-95466"))

			// clean up
			Expect(k8sClient.Delete(ctx, service)).Should(Succeed())
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
