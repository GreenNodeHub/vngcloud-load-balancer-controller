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

package glbc_controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

var _ = Describe("GlobalLoadBalancerConfig Controller", func() {

	AfterEach(func() {
		expectNoGLBs()
		expectNoGLBCObjects()
	})

	Context("Create flow", func() {
		It("should create LB, pool, and listener in mock backend and populate status", func() {
			glbc := newGLBCResource("test-glbc-create", "default")
			Expect(k8sClient.Create(ctx, glbc)).Should(Succeed())

			// Wait for finalizer to be added
			Eventually(func(g Gomega) {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-create", Namespace: "default"}, updated)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(updated.Finalizers).Should(ContainElement(domain.GlbcFinalizer))
			}, timeout*2, interval).Should(Succeed())

			// Wait for status to be populated with LB, pool, listener and Ready condition
			Eventually(func(g Gomega) {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-create", Namespace: "default"}, updated)
				g.Expect(err).ShouldNot(HaveOccurred())

				// LoadBalancerId must be set
				g.Expect(updated.Status.LoadBalancerId).ShouldNot(BeNil())
				g.Expect(*updated.Status.LoadBalancerId).ShouldNot(BeEmpty())

				// Exactly one pool
				g.Expect(updated.Status.CreatedPools).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedPools[0].Name).Should(Equal("test-pool"))
				g.Expect(updated.Status.CreatedPools[0].Id).ShouldNot(BeEmpty())

				// Exactly one listener
				g.Expect(updated.Status.CreatedListeners).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedListeners[0].Name).Should(Equal("test-listener"))
				g.Expect(updated.Status.CreatedListeners[0].Id).ShouldNot(BeEmpty())

				// Ready condition must be True
				condition := meta.FindStatusCondition(updated.Status.Conditions, v1alpha1.GLBCConditionTypeReady)
				g.Expect(condition).ShouldNot(BeNil())
				g.Expect(condition.Status).Should(Equal(metav1.ConditionTrue))

				// Verify mock backend has the LB
				lb, err := vngcloudRepo.GetGlobalLoadBalancerByID(ctx, *updated.Status.LoadBalancerId)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(lb).ShouldNot(BeNil())
			}, timeout*4, interval).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, glbc)).Should(Succeed())
		})
	})

	Context("Delete flow (full -- controller owns LB)", func() {
		It("should delete LB entirely from mock backend", func() {
			glbc := newGLBCResource("test-glbc-delete-full", "default")
			Expect(k8sClient.Create(ctx, glbc)).Should(Succeed())

			// Wait for status to be populated
			var lbID string
			Eventually(func(g Gomega) {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-delete-full", Namespace: "default"}, updated)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(updated.Status.LoadBalancerId).ShouldNot(BeNil())
				g.Expect(*updated.Status.LoadBalancerId).ShouldNot(BeEmpty())
				g.Expect(updated.Status.CreatedPools).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedListeners).Should(HaveLen(1))
				lbID = *updated.Status.LoadBalancerId
			}, timeout*4, interval).Should(Succeed())

			// Delete the GLBC
			Expect(k8sClient.Delete(ctx, glbc)).Should(Succeed())

			// Wait for K8s object to be gone
			Eventually(func() bool {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-delete-full", Namespace: "default"}, updated)
				return errors.IsNotFound(err)
			}, timeout*4, interval).Should(BeTrue(), "Expected GLBC to be deleted from K8s")

			// Wait for mock backend to have no LB
			Eventually(func() error {
				_, err := vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbID)
				return err
			}, timeout*4, interval).Should(Equal(domain.ErrorNotFound), "Expected LB to be deleted from mock backend")
		})
	})

	Context("Delete flow (partial -- shared LB)", func() {
		It("should clean up only owned resources, leave shared ones", func() {
			// Create GLBC-A (creates the LB)
			glbcA := newGLBCResource("test-glbc-a", "default")
			Expect(k8sClient.Create(ctx, glbcA)).Should(Succeed())

			// Wait for GLBC-A status with LoadBalancerId
			var lbID string
			var glbcAPoolID string
			var glbcAListenerID string
			Eventually(func(g Gomega) {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-a", Namespace: "default"}, updated)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(updated.Status.LoadBalancerId).ShouldNot(BeNil())
				g.Expect(*updated.Status.LoadBalancerId).ShouldNot(BeEmpty())
				g.Expect(updated.Status.CreatedPools).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedListeners).Should(HaveLen(1))
				lbID = *updated.Status.LoadBalancerId
				glbcAPoolID = updated.Status.CreatedPools[0].Id
				glbcAListenerID = updated.Status.CreatedListeners[0].Id
			}, timeout*4, interval).Should(Succeed())

			// Create GLBC-B sharing the same LB
			glbcB := newGLBCSharedResource("test-glbc-b", "default", lbID)
			Expect(k8sClient.Create(ctx, glbcB)).Should(Succeed())

			// Wait for GLBC-B status populated
			var glbcBPoolID string
			var glbcBListenerID string
			Eventually(func(g Gomega) {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-b", Namespace: "default"}, updated)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(updated.Status.CreatedPools).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedPools[0].Name).Should(Equal("test-pool-shared"))
				g.Expect(updated.Status.CreatedListeners).Should(HaveLen(1))
				g.Expect(updated.Status.CreatedListeners[0].Name).Should(Equal("test-listener-shared"))
				glbcBPoolID = updated.Status.CreatedPools[0].Id
				glbcBListenerID = updated.Status.CreatedListeners[0].Id
			}, timeout*4, interval).Should(Succeed())

			// Delete GLBC-A (partial delete — GLBC-B still references the LB)
			Expect(k8sClient.Delete(ctx, glbcA)).Should(Succeed())

			// Wait for GLBC-A to be fully gone from K8s
			Eventually(func() bool {
				updated := &v1alpha1.GlobalLoadBalancerConfig{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test-glbc-a", Namespace: "default"}, updated)
				return errors.IsNotFound(err)
			}, timeout*4, interval).Should(BeTrue(), "Expected GLBC-A to be deleted from K8s")

			// Assert mock backend state: LB still exists
			lb, err := vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbID)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(lb).ShouldNot(BeNil())

			// Assert GLBC-B's pool still exists
			pools, err := vngcloudRepo.ListGlobalPools(ctx, lbID)
			Expect(err).ShouldNot(HaveOccurred())
			poolIDs := make([]string, len(pools.Items))
			for i, p := range pools.Items {
				poolIDs[i] = p.ID
			}
			Expect(poolIDs).Should(ContainElement(glbcBPoolID), "GLBC-B's pool should still exist")
			Expect(poolIDs).ShouldNot(ContainElement(glbcAPoolID), "GLBC-A's pool should be deleted")

			// Assert GLBC-B's listener still exists
			listeners, err := vngcloudRepo.ListGlobalListeners(ctx, lbID)
			Expect(err).ShouldNot(HaveOccurred())
			listenerIDs := make([]string, len(listeners.Items))
			for i, l := range listeners.Items {
				listenerIDs[i] = l.ID
			}
			Expect(listenerIDs).Should(ContainElement(glbcBListenerID), "GLBC-B's listener should still exist")
			Expect(listenerIDs).ShouldNot(ContainElement(glbcAListenerID), "GLBC-A's listener should be deleted")

			// Cleanup GLBC-B
			Expect(k8sClient.Delete(ctx, glbcB)).Should(Succeed())
		})
	})

})
