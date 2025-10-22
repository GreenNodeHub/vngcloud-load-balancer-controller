package service_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type mockEndpointResolver struct {
	utils.EndpointResolver
}

func (m *mockEndpointResolver) ResolvePodEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...utils.EndpointResolveOption) ([]utils.EndpointAddress, error) {
	return []utils.EndpointAddress{
		{
			IP:   "10.0.1.10",
			Port: 8080,
		},
	}, nil
}

func (m *mockEndpointResolver) ResolveNodePortEndpoints(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString, opts ...utils.EndpointResolveOption) ([]utils.EndpointAddress, error) {
	return []utils.EndpointAddress{
		{
			IP:   "10.0.1.10",
			Port: 30080,
		},
	}, nil
}

func (m *mockEndpointResolver) GetListTargetPort(ctx context.Context, svcKey types.NamespacedName, port intstr.IntOrString) ([]int, error) {
	return []int{8080}, nil
}

func TestBuildDefaultSecurityGroupRule(t *testing.T) {
	// Create a mock annotation parser
	parser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")

	// Create a mock service with ports
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Annotations: map[string]string{
				"service.beta.kubernetes.io/vngcloud-loadbalancer-type": "layer4",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
				{
					Name:       "https",
					Port:       443,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8443),
				},
			},
		},
	}

	// Create a mock task
	task := &defaultModelBuildTask{
		annotationParser: parser,
		service:          service,
		vlbConfig: &v1alpha1.VngcloudLoadBalancerConfig{
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{},
		},
		nameHelper:       utils.NewNameHelper("test-cluster", "service", "default", "test-service"),
		cniMode:          utils.CiliumNativeRouting,
		endpointResolver: &mockEndpointResolver{},
		networkId:        "network-123",
		subnetId:         "subnet-123",
		subnetCIDR:       "10.0.0.0/24",
		zone:             "vn-south-1a",
	}

	t.Run("Test buildDefaultSecurityGroupRule with TCP ports", func(t *testing.T) {
		result, err := task.buildDefaultSecurityGroupRule(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should have rules for:
		// 1. Health check port (if configured)
		// 2. Pool members (from pods/nodes)
		// 3. ICMP rules for UDP rules (if any)

		// Check that we have at least the basic rules
		assert.NotEmpty(t, result)
	})

	t.Run("Test buildDefaultSecurityGroupRule with UDP ports", func(t *testing.T) {
		// Create a service with UDP ports
		udpService := service.DeepCopy()
		udpService.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "dns",
				Port:       53,
				Protocol:   corev1.ProtocolUDP,
				TargetPort: intstr.FromInt(53),
			},
		}

		// Update task with UDP service
		task.service = udpService

		result, err := task.buildDefaultSecurityGroupRule(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should have ICMP rules for UDP rules
		assert.NotEmpty(t, result)
	})

	t.Run("Test buildDefaultSecurityGroupRule with health check port", func(t *testing.T) {
		// Create a service with health check annotation
		annotatedService := service.DeepCopy()
		annotatedService.Annotations["service.beta.kubernetes.io/vngcloud-loadbalancer-health-check-port"] = "8080"

		// Update task with annotated service
		task.service = annotatedService

		result, err := task.buildDefaultSecurityGroupRule(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should have health check rule
		assert.NotEmpty(t, result)
	})

	t.Run("Test buildDefaultSecurityGroupRule with instance target type", func(t *testing.T) {
		// Create a service with instance target type
		instanceService := service.DeepCopy()
		instanceService.Spec.Type = corev1.ServiceTypeNodePort

		// Update task with instance service
		task.service = instanceService

		result, err := task.buildDefaultSecurityGroupRule(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should have rules for NodePort targets
		assert.NotEmpty(t, result)
	})

	t.Run("Test buildDefaultSecurityGroupRule with empty service ports", func(t *testing.T) {
		// Create a service with no ports
		noPortService := service.DeepCopy()
		noPortService.Spec.Ports = []corev1.ServicePort{}

		// Update task with no port service
		task.service = noPortService

		result, err := task.buildDefaultSecurityGroupRule(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Should return empty rules
		assert.Empty(t, result)
	})
}

func TestEnsureUniqueSecgroupRules(t *testing.T) {
	tests := []struct {
		name     string
		rules    []v1alpha1.SecurityGroupRule
		expected []v1alpha1.SecurityGroupRule
	}{
		{
			name: "no duplicates",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
		},
		{
			name: "with duplicates",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
		},
		{
			name: "multiple duplicates",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 53,
					ToPort:   53,
					CIDR:     "10.0.1.0/24",
				},
			},
		},
		{
			name: "different protocols with same ports and cidr",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolUDP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
			},
		},
		{
			name: "same protocol, different ports",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 443,
					ToPort:   443,
					CIDR:     "10.0.0.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 443,
					ToPort:   443,
					CIDR:     "10.0.0.0/24",
				},
			},
		},
		{
			name: "same protocol, same ports, different cidrs",
			rules: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.1.0/24",
				},
			},
			expected: []v1alpha1.SecurityGroupRule{
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.0.0/24",
				},
				{
					Protocol: networkv2.SecgroupRuleProtocolTCP,
					FromPort: 80,
					ToPort:   80,
					CIDR:     "10.0.1.0/24",
				},
			},
		},
		{
			name:     "empty slice",
			rules:    []v1alpha1.SecurityGroupRule{},
			expected: []v1alpha1.SecurityGroupRule{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock struct that implements the interface
			task := &defaultModelBuildTask{}

			result := task.ensureUniqueSecgroupRules(tt.rules)

			assert.Equal(t, tt.expected, result)
		})
	}
}
