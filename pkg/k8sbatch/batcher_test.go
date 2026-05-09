package k8sbatch_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
