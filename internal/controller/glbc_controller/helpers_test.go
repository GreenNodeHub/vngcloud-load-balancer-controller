package glbc_controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeout  = time.Second * 5
	interval = time.Millisecond * 250
)

// ============================================================================
// Helper functions to verify clean state
// ============================================================================

func expectNoGLBs() {
	Eventually(func() int {
		glbs, err := vngcloudRepo.ListGlobalLoadBalancers(ctx, nil)
		if err != nil || glbs == nil {
			return -1
		}
		count := len(glbs.Items)
		if count > 0 {
			GinkgoWriter.Printf("WARNING: Found %d global load balancers still present:\n", count)
			for _, glb := range glbs.Items {
				GinkgoWriter.Printf("   - %s (%s)\n", glb.Name, glb.ID)
			}
		}
		return count
	}, timeout*4, interval).Should(Equal(0), "Expected no global load balancers")
}

func expectNoGLBCObjects() {
	Eventually(func() int {
		glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
		err := k8sClient.List(ctx, glbcList)
		if err != nil {
			return -1
		}
		count := len(glbcList.Items)
		if count > 0 {
			GinkgoWriter.Printf("WARNING: Found %d GLBCs still present:\n", count)
			for _, glbc := range glbcList.Items {
				GinkgoWriter.Printf("   - %s/%s\n", glbc.Namespace, glbc.Name)
			}
		}
		return count
	}, timeout*4, interval).Should(Equal(0), "Expected no GLBCs in any namespace")
}

// ============================================================================
// Helper functions to create GlobalLoadBalancerConfig test resources
// ============================================================================

func newGLBCResource(name, namespace string) *v1alpha1.GlobalLoadBalancerConfig {
	spec := vngcloud_mocks.MockGLBCMinimalSpec()
	return &v1alpha1.GlobalLoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "GlobalLoadBalancerConfig",
			APIVersion: "vks.vngcloud.vn/v1alpha1",
		},
		Spec: spec,
	}
}

func newGLBCSharedResource(name, namespace, lbID string) *v1alpha1.GlobalLoadBalancerConfig {
	spec := vngcloud_mocks.MockGLBCSharedSpec()
	spec.LoadBalancerId = &lbID
	return &v1alpha1.GlobalLoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "GlobalLoadBalancerConfig",
			APIVersion: "vks.vngcloud.vn/v1alpha1",
		},
		Spec: spec,
	}
}
