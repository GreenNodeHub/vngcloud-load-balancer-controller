package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
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
	}, timeout*4, interval).Should(Equal(0), "Expected no load balancers")
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
	}, timeout*4, interval).Should(Equal(0), "Expected no security groups")
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
	}, timeout*4, interval).Should(Equal(0), "Expected no services in any namespace")
}

func expectNoVLBCs() {
	Eventually(func() int {
		vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
		err := k8sClient.List(ctx, vlbcList)
		if err != nil {
			return -1
		}
		count := len(vlbcList.Items)
		if count > 0 {
			GinkgoWriter.Printf("⚠️  Found %d VLBCs still present:\n", count)
			for _, vlbc := range vlbcList.Items {
				GinkgoWriter.Printf("   - %s/%s\n", vlbc.Namespace, vlbc.Name)
			}
		}
		return count
	}, timeout*4, interval).Should(Equal(0), "Expected no VLBCs in any namespace")
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
	}, timeout*4, interval).Should(Equal(0), "Expected no endpoints in any namespace")
}

// ============================================================================
// Helper functions to cleanup resources
// ============================================================================

func cleanupAllServices(namespace string) {
	serviceList := &corev1.ServiceList{}
	err := k8sClient.List(ctx, serviceList, client.InNamespace(namespace))
	if err != nil {
		return
	}
	for _, svc := range serviceList.Items {
		k8sClient.Delete(ctx, &svc)
	}
}

func cleanupAllEndpoints(namespace string) {
	endpointList := &corev1.EndpointsList{}
	err := k8sClient.List(ctx, endpointList, client.InNamespace(namespace))
	if err != nil {
		return
	}
	for _, ep := range endpointList.Items {
		k8sClient.Delete(ctx, &ep)
	}
}

func cleanupAllVLBCs(namespace string) {
	vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
	err := k8sClient.List(ctx, vlbcList, client.InNamespace(namespace))
	if err != nil {
		return
	}
	for _, vlbc := range vlbcList.Items {
		k8sClient.Delete(ctx, &vlbc)
	}
}

func cleanupAllNSGs(namespace string) {
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	err := k8sClient.List(ctx, nsgList, client.InNamespace(namespace))
	if err != nil {
		return
	}
	for _, nsg := range nsgList.Items {
		k8sClient.Delete(ctx, &nsg)
	}
}

// ============================================================================
// Helper functions to get resources
// ============================================================================

func getServiceResource(serviceName, namespace string) (*corev1.Service, error) {
	service := &corev1.Service{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: serviceName, Namespace: namespace}, service)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func getVLBCListForService(serviceName, namespace string) (*v1alpha1.VngcloudLoadBalancerConfigList, error) {
	vlbcList := &v1alpha1.VngcloudLoadBalancerConfigList{}
	err := k8sClient.List(ctx, vlbcList, client.InNamespace(namespace), client.MatchingLabels{
		consts.LabelOwnerResourceName: serviceName,
		consts.LabelOwnerResourceType: "Service",
	})
	return vlbcList, err
}

func getVLBCForService(serviceName, namespace string) (*v1alpha1.VngcloudLoadBalancerConfig, error) {
	vlbcList, err := getVLBCListForService(serviceName, namespace)
	if err != nil {
		return nil, err
	}
	if len(vlbcList.Items) == 0 {
		return nil, nil
	}
	return &vlbcList.Items[0], nil
}

func getNSGListForService(serviceName, namespace string) (*v1alpha1.NodeSecurityGroupList, error) {
	nsgList := &v1alpha1.NodeSecurityGroupList{}
	err := k8sClient.List(ctx, nsgList, client.InNamespace(namespace), client.MatchingLabels{
		consts.LabelOwnerResourceName: serviceName,
		consts.LabelOwnerResourceType: "Service",
	})
	return nsgList, err
}

// ============================================================================
// Helper functions to wait for resources
// ============================================================================

func waitForLoadBalancerId(vlbcName, namespace string) (string, error) {
	var loadbalancerId string

	success := Eventually(func() bool {
		vlbc := &v1alpha1.VngcloudLoadBalancerConfig{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: vlbcName, Namespace: namespace}, vlbc)
		if err != nil {
			return false
		}
		if vlbc.Status.LoadBalancerId != nil {
			loadbalancerId = *vlbc.Status.LoadBalancerId
			return loadbalancerId != ""
		}
		return false
	}, timeout*2, interval).Should(BeTrue())

	if !success {
		return "", nil
	}
	return loadbalancerId, nil
}

// ============================================================================
// Helper functions to create test resources
// ============================================================================

func newServiceResource(name, namespace string) *corev1.Service {
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
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					Protocol: corev1.ProtocolTCP,
					NodePort: 30080,
				},
			},
			Selector: map[string]string{
				"app": "test",
			},
		},
	}
}

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
