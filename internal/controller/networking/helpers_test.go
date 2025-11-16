package networking

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// ============================================================================
// Helper functions to verify clean state
// ============================================================================

func expectNoLoadBalancers() {
	Eventually(func() int {
		lbs, err := vngcloudRepo.ListLoadBalancers(ctx, []string{})
		if err != nil || lbs == nil {
			return -1
		}
		count := len(lbs.Items)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d load balancers still present:\n", count)
			for _, lb := range lbs.Items {
				GinkgoWriter.Printf("   - %s\n", lb.Name)
			}
		}
		return count
	}, timeout, interval).Should(Equal(0), "Expected no load balancers")
}

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
	}, timeout, interval).Should(Equal(0), "Expected no security groups")
}

func expectNoServices() {
	Eventually(func() int {
		serviceList := &corev1.ServiceList{}
		err := k8sClient.List(ctx, serviceList)
		if err != nil {
			return -1
		}
		// Filter out the default kubernetes service
		filteredServices := []corev1.Service{}
		for _, svc := range serviceList.Items {
			if svc.Namespace == "default" && svc.Name == "kubernetes" {
				continue
			}
			filteredServices = append(filteredServices, svc)
		}
		count := len(filteredServices)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d services still present:\n", count)
			for _, svc := range filteredServices {
				GinkgoWriter.Printf("   - %s/%s (Type: %s)\n", svc.Namespace, svc.Name, svc.Spec.Type)
			}
		}
		return count
	}, timeout, interval).Should(Equal(0), "Expected no services in any namespace")
}

func expectNoIngresses() {
	Eventually(func() int {
		ingressList := &networkingv1.IngressList{}
		err := k8sClient.List(ctx, ingressList)
		if err != nil {
			return -1
		}
		if len(ingressList.Items) > 0 {
			GinkgoWriter.Printf("⚠️  Found %d ingresses still present:\n", len(ingressList.Items))
			for _, ingress := range ingressList.Items {
				GinkgoWriter.Printf("   - %s/%s\n", ingress.Namespace, ingress.Name)
			}
		}
		return len(ingressList.Items)
	}, timeout, interval).Should(Equal(0), "Expected no ingresses in any namespace")
}

func expectNoLBCs() {
	Eventually(func() int {
		lbcList := &v1alpha1.LoadBalancerConfigList{}
		err := k8sClient.List(ctx, lbcList)
		if err != nil {
			return -1
		}
		count := len(lbcList.Items)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d LBCs still present:\n", count)
			for _, lbc := range lbcList.Items {
				GinkgoWriter.Printf("   - %s/%s\n", lbc.Namespace, lbc.Name)
			}
		}
		return count
	}, timeout, interval).Should(Equal(0), "Expected no LBCs in any namespace")
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
	}, timeout, interval).Should(Equal(0), "Expected no NSGs in any namespace")
}

func expectNoEndpoints() {
	Eventually(func() int {
		endpointList := &corev1.EndpointsList{}
		err := k8sClient.List(ctx, endpointList)
		if err != nil {
			return -1
		}
		// Filter out the default kubernetes endpoint
		filteredEndpoints := []corev1.Endpoints{}
		for _, ep := range endpointList.Items {
			if ep.Namespace == "default" && ep.Name == "kubernetes" {
				continue
			}
			filteredEndpoints = append(filteredEndpoints, ep)
		}
		count := len(filteredEndpoints)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d endpoints still present:\n", count)
			for _, ep := range filteredEndpoints {
				GinkgoWriter.Printf("   - %s/%s\n", ep.Namespace, ep.Name)
			}
		}
		return count
	}, timeout, interval).Should(Equal(0), "Expected no endpoints in any namespace")
}

// ============================================================================
// Helper functions to get resources
// ============================================================================

func listLbcByIngress(name, namespace string) (*v1alpha1.LoadBalancerConfigList, error) {
	lbcList := &v1alpha1.LoadBalancerConfigList{}
	err := k8sClient.List(ctx, lbcList, client.InNamespace(namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: name,
		domain.LabelOwnerResourceType: "Ingress",
	})
	return lbcList, err
}

func listNsgByIngress(name, namespace string) (*v1alpha1.NodeSecurityGroupList, error) {
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	err := k8sClient.List(ctx, nsgList, client.InNamespace(namespace), client.MatchingLabels{
		domain.LabelOwnerResourceName: name,
		domain.LabelOwnerResourceType: "Ingress",
	})
	return nsgList, err
}

// ============================================================================
// Helper functions to create test resources
// ============================================================================

var _ = newIngressResource("placeholder", "placeholder")

func newIngressResource(name, namespace string) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"annd2.space": "test",
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ptr.To("vngcloud"),
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: "kubernetes",
					Port: networkingv1.ServiceBackendPort{
						Number: 443,
					},
				},
			},
		},
	}
}

var _ = newServiceNodePortResource("placeholder", "placeholder")

func newServiceNodePortResource(name, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
}

var _ = newEndpointResource("placeholder", "placeholder")

func newEndpointResource(name, namespace string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP:       "172.172.172.0",
						Hostname: "test",
						NodeName: ptr.To("mock-node-1"),
						TargetRef: &corev1.ObjectReference{
							Kind:      "Pod",
							Namespace: namespace,
							Name:      "pod-1",
						},
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Name:        "http",
						Port:        80,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr.To("http"),
					},
				},
			},
		},
	}
}

// removeFisrt removes the first occurrence of a value from a slice
func removeFisrt[T comparable](slice []T, value T) []T {
	for i, v := range slice {
		if v == value {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
