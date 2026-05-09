package k8sbatch_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8sbatch"
)

var _ = Describe("Batcher", func() {
	Describe("New", func() {
		It("returns a non-nil Batcher with Pending() == 0", func() {
			b := k8sbatch.New(k8sClient)
			Expect(b).NotTo(BeNil())
			Expect(b.Pending()).To(Equal(0))
		})
	})
})

var _ = Describe("Batcher Flush — single status mutator", func() {
	var ns string

	BeforeEach(func() {
		ns = fmt.Sprintf("k8sbatch-%d", GinkgoRandomSeed())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	It("issues one Status patch when a mutator returns true", func() {
		lbc := newTestLBC(ns, "lbc-1")
		Expect(k8sClient.Create(ctx, lbc)).To(Succeed())

		b := k8sbatch.New(k8sClient)
		k8sbatch.MutateStatus(b, lbc, func(o *v1alpha1.LoadBalancerConfig) bool {
			o.Status.LastReconcileMessage = "patched-by-batcher"
			return true
		})

		Expect(b.Flush(ctx)).To(Succeed())
		Expect(b.Pending()).To(Equal(0))

		got := &v1alpha1.LoadBalancerConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lbc), got)).To(Succeed())
		Expect(got.Status.LastReconcileMessage).To(Equal("patched-by-batcher"))
	})
})
