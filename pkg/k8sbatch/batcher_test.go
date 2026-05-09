package k8sbatch_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		ns = fmt.Sprintf("k8sbatch-%d-%d", GinkgoRandomSeed(), CurrentSpecReport().LeafNodeLocation.LineNumber)
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

	It("does not patch when the mutator returns false", func() {
		lbc := newTestLBC(ns, "lbc-noop")
		Expect(k8sClient.Create(ctx, lbc)).To(Succeed())

		// Capture the original ResourceVersion. If no patch happens, RV stays.
		original := &v1alpha1.LoadBalancerConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lbc), original)).To(Succeed())
		originalRV := original.ResourceVersion

		b := k8sbatch.New(k8sClient)
		k8sbatch.MutateStatus(b, lbc, func(o *v1alpha1.LoadBalancerConfig) bool {
			return false // no change
		})

		Expect(b.Flush(ctx)).To(Succeed())
		Expect(b.Pending()).To(Equal(0))

		got := &v1alpha1.LoadBalancerConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lbc), got)).To(Succeed())
		Expect(got.ResourceVersion).To(Equal(originalRV), "expected no patch and therefore unchanged ResourceVersion")
	})
})

var _ = Describe("Batcher Flush — multiple mutators on one object", func() {
	var ns string

	BeforeEach(func() {
		ns = fmt.Sprintf("k8sbatch-multi-%d", GinkgoRandomSeed())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	It("applies status mutators in queue order and issues one patch", func() {
		lbc := newTestLBC(ns, "lbc-order")
		Expect(k8sClient.Create(ctx, lbc)).To(Succeed())

		b := k8sbatch.New(k8sClient)
		// Mutator A sets the message to "first".
		k8sbatch.MutateStatus(b, lbc, func(o *v1alpha1.LoadBalancerConfig) bool {
			o.Status.LastReconcileMessage = "first"
			return true
		})
		// Mutator B asserts A ran, then overwrites with "second".
		k8sbatch.MutateStatus(b, lbc, func(o *v1alpha1.LoadBalancerConfig) bool {
			Expect(o.Status.LastReconcileMessage).To(Equal("first"),
				"mutator B should observe mutator A's change")
			o.Status.LastReconcileMessage = "second"
			return true
		})

		Expect(b.Flush(ctx)).To(Succeed())

		got := &v1alpha1.LoadBalancerConfig{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(lbc), got)).To(Succeed())
		Expect(got.Status.LastReconcileMessage).To(Equal("second"))
	})
})

var _ = Describe("Batcher Flush — NotFound on GET", func() {
	var ns string

	BeforeEach(func() {
		ns = fmt.Sprintf("k8sbatch-nf-%d", GinkgoRandomSeed())
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	It("returns the NotFound error and leaves the entry queued", func() {
		// Note: never created in the API server.
		ghost := newTestLBC(ns, "does-not-exist")

		b := k8sbatch.New(k8sClient)
		k8sbatch.MutateStatus(b, ghost, func(o *v1alpha1.LoadBalancerConfig) bool {
			o.Status.LastReconcileMessage = "should-never-land"
			return true
		})
		Expect(b.Pending()).To(Equal(1))

		err := b.Flush(ctx)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"expected the joined error to unwrap to a NotFound; got: %v", err)
		Expect(b.Pending()).To(Equal(1), "failed entry should remain queued")
	})
})
