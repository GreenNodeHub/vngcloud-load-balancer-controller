package nsg_controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

const (
	timeout  = time.Second * 5
	interval = time.Millisecond * 250
)

// ============================================================================
// Helper functions to verify clean state
// ============================================================================

func expectNoSecurityGroups() {
	Eventually(func() int {
		secgroups, err := vngcloudRepo.ListSecurityGroups(ctx)
		if err != nil || secgroups == nil {
			return -1
		}
		count := len(secgroups.Items)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d security groups still present:\n", count)
			for _, sg := range secgroups.Items {
				GinkgoWriter.Printf("   - %s\n", sg.Name)
			}
		}
		return count
	}, timeout*4, interval).Should(Equal(0), "Expected no security groups")
}

func expectNoNSGs() {
	Eventually(func() int {
		nsgList := &v1alpha1.NodeSecurityGroupList{}
		err := k8sClient.List(ctx, nsgList)
		if err != nil {
			return -1
		}
		count := len(nsgList.Items)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d NSGs still present:\n", count)
			for _, nsg := range nsgList.Items {
				GinkgoWriter.Printf("   - %s/%s\n", nsg.Namespace, nsg.Name)
			}
		}
		return count
	}, timeout*4, interval).Should(Equal(0), "Expected no NSGs in any namespace")
}
