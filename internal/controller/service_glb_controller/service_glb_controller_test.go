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

package service_glb_controller

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

const (
	svcGLBTimeout  = time.Second * 30
	svcGLBInterval = time.Second * 1
)

var _ = Describe("ServiceGLB Controller", func() {

	Context("Create flow", func() {
		It("Should create GLBC when Service has glb.vks.vngcloud.vn/enable=true", func() {
			// Create a unique namespace for isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-svc-create-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			svcName := "test-svc"
			svc := newServiceWithGLBAnnotation(svcName, ns.Name, []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 30080,
					Protocol: corev1.ProtocolTCP,
				},
			})
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, svc)
			})

			// Eventually: GLBC appears with correct owner labels
			var foundGLBC *v1alpha1.GlobalLoadBalancerConfig
			Eventually(func(g Gomega) {
				// Refresh Service to get latest UID
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())

				glbc, err := findGLBCByServiceOwnerLabels(ctx, k8sClient, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(glbc).ShouldNot(BeNil())
				foundGLBC = glbc
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())

			// Assert owner labels
			Expect(foundGLBC.Labels[domain.LabelOwnerResourceKind]).To(Equal(domain.KindService),
				"Expected owner kind to be Service")
			Expect(foundGLBC.Labels[domain.LabelOwnerResourceName]).To(Equal(svcName),
				"Expected owner name to match service name")

			// Assert spec: 1 pool, 1 listener
			Expect(foundGLBC.Spec.GlobalPools).To(HaveLen(1),
				"Expected 1 global pool")
			pool := foundGLBC.Spec.GlobalPools[0]
			Expect(strings.HasPrefix(pool.Name, "pool-")).To(BeTrue(),
				"Expected pool name to start with 'pool-', got: %s", pool.Name)

			Expect(foundGLBC.Spec.GlobalListeners).To(HaveLen(1),
				"Expected 1 global listener")
			listener := foundGLBC.Spec.GlobalListeners[0]
			Expect(listener.ProtocolPort).To(Equal(80),
				"Expected listener port 80, got: %d", listener.ProtocolPort)

			// Assert pool has members (at least 1 member group)
			Expect(pool.PoolMembers).To(HaveLen(1),
				"Expected 1 pool member group (one per region)")
			pm := pool.PoolMembers[0]
			// Assert member group has at least 1 individual member with the test node IP
			Expect(pm.Members).NotTo(BeEmpty(),
				"Expected pool member group to have members")
			var addresses []string
			for _, m := range pm.Members {
				addresses = append(addresses, m.Address)
			}
			Expect(addresses).To(ContainElement("10.0.0.1"),
				"Expected test node IP 10.0.0.1 in pool members, got: %v", addresses)

			// Assert Service finalizer was added
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				hasFinalizer := false
				for _, f := range svc.Finalizers {
					if f == domain.ServiceGLBFinalizer {
						hasFinalizer = true
						break
					}
				}
				g.Expect(hasFinalizer).To(BeTrue(),
					"Expected Service to have GLB finalizer %s", domain.ServiceGLBFinalizer)
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())
		})
	})

	Context("Update flow", func() {
		It("Should update GLBC spec when Service ports change", func() {
			// Create a unique namespace for isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-svc-update-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			svcName := "test-svc-update"
			svc := newServiceWithGLBAnnotation(svcName, ns.Name, []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 30180,
					Protocol: corev1.ProtocolTCP,
				},
			})
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, svc)
			})

			// Wait for GLBC to appear with 1 pool/listener
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())

				glbc, err := findGLBCByServiceOwnerLabels(ctx, k8sClient, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(glbc).ShouldNot(BeNil())
				g.Expect(glbc.Spec.GlobalPools).To(HaveLen(1))
				g.Expect(glbc.Spec.GlobalListeners).To(HaveLen(1))
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())

			// Patch Service to add second port 443
			updatedSvc := svc.DeepCopy()
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, updatedSvc)).To(Succeed())
			updatedSvc.Spec.Ports = []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 30180,
					Protocol: corev1.ProtocolTCP,
				},
				{
					Name:     "https",
					Port:     443,
					NodePort: 30443,
					Protocol: corev1.ProtocolTCP,
				},
			}
			Expect(k8sClient.Update(ctx, updatedSvc)).To(Succeed())

			// Eventually: GLBC has 2 pools and 2 listeners
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())

				glbc, err := findGLBCByServiceOwnerLabels(ctx, k8sClient, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(glbc).ShouldNot(BeNil())
				g.Expect(glbc.Spec.GlobalPools).To(HaveLen(2),
					"Expected 2 global pools after service port update")
				g.Expect(glbc.Spec.GlobalListeners).To(HaveLen(2),
					"Expected 2 global listeners after service port update")
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())
		})
	})

	Context("Delete flow", func() {
		It("Should delete GLBC when enable annotation is removed", func() {
			// Create a unique namespace for isolation
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-svc-delete-",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, ns)
			})

			svcName := "test-svc-delete"
			svc := newServiceWithGLBAnnotation(svcName, ns.Name, []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					NodePort: 30280,
					Protocol: corev1.ProtocolTCP,
				},
			})
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, svc)
			})

			// Wait for GLBC to appear
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())

				glbc, err := findGLBCByServiceOwnerLabels(ctx, k8sClient, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(glbc).ShouldNot(BeNil())
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())

			// Remove the glb.vks.vngcloud.vn/enable annotation
			patchSvc := svc.DeepCopy()
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, patchSvc)).To(Succeed())
			patchSvc.Annotations = map[string]string{}
			Expect(k8sClient.Update(ctx, patchSvc)).To(Succeed())

			// Eventually: GLBC is gone (list by owner labels returns nil)
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())

				glbc, err := findGLBCByServiceOwnerLabels(ctx, k8sClient, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(glbc).To(BeNil(),
					"Expected GLBC to be deleted after annotation removal")
			}, svcGLBTimeout*2, svcGLBInterval).Should(Succeed())

			// Wait for Service finalizer to be removed
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ns.Name}, svc)
				g.Expect(err).ShouldNot(HaveOccurred())
				hasFinalizer := false
				for _, f := range svc.Finalizers {
					if f == domain.ServiceGLBFinalizer {
						hasFinalizer = true
						break
					}
				}
				g.Expect(hasFinalizer).To(BeFalse(),
					"Expected Service GLB finalizer to be removed")
			}, svcGLBTimeout, svcGLBInterval).Should(Succeed())
		})
	})

})
