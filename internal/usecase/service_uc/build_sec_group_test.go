package service_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func TestBuildDefaultSecurityGroupRule(t *testing.T) {
	tests := []struct {
		name                     string
		service                  *corev1.Service
		cniMode                  utils.CNIType
		resolveNodePortEndpoints []utils.EndpointAddress
		resolvePodEndpoints      []utils.EndpointAddress
		getListTargetPort        []int
		expectedRules            []v1alpha1.NodeSecurityGroupRule
		expectError              bool
	}{
		{
			name: "TCP service with instance target type and node port endpoints",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
							NodePort: 30080,
						},
					},
				},
			},
			cniMode: utils.CalicoOverlay,
			resolveNodePortEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.0.0.1",
					Port: 30080,
					Name: "node-1",
				},
				{
					IP:   "10.0.0.2",
					Port: 30080,
					Name: "node-2",
				},
			},
			expectedRules: []v1alpha1.NodeSecurityGroupRule{
				{
					Protocol:    v2.SecgroupRuleProtocolTCP,
					FromPort:    30080,
					ToPort:      30080,
					CIDR:        "192.168.1.0/24",
					Description: "Allow load balancer access to port 30080",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "0.0.0.0/0",
					Description: "Default egress security group rule for IPv4",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "::/0",
					Description: "Default egress security group rule for IPv6",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv6,
				},
			},
			expectError: false,
		},
		{
			name: "UDP service with instance target type should include ICMP rules",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "dns",
							Port:     53,
							Protocol: corev1.ProtocolUDP,
							NodePort: 30053,
						},
					},
				},
			},
			cniMode: utils.CalicoOverlay,
			resolveNodePortEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.0.0.1",
					Port: 30053,
					Name: "node-1",
				},
			},
			expectedRules: []v1alpha1.NodeSecurityGroupRule{
				{
					Protocol:    v2.SecgroupRuleProtocolUDP,
					FromPort:    30053,
					ToPort:      30053,
					CIDR:        "192.168.1.0/24",
					Description: "Allow load balancer access to port 30053",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolICMP,
					FromPort:    30053,
					ToPort:      30053,
					CIDR:        "192.168.1.0/24",
					Description: "Allow ICMP for health check UDP port",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "0.0.0.0/0",
					Description: "Default egress security group rule for IPv4",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "::/0",
					Description: "Default egress security group rule for IPv6",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv6,
				},
			},
			expectError: false,
		},
		{
			name: "Service with health check port annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:      string(domain.TargetTypeInstance),
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixHealthcheckPort: "8080",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http",
							Port:     80,
							Protocol: corev1.ProtocolTCP,
							NodePort: 30080,
						},
					},
				},
			},
			cniMode: utils.CalicoOverlay,
			resolveNodePortEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.0.0.1",
					Port: 30080,
					Name: "node-1",
				},
			},
			expectedRules: []v1alpha1.NodeSecurityGroupRule{
				{
					Protocol:    v2.SecgroupRuleProtocolTCP,
					FromPort:    8080,
					ToPort:      8080,
					CIDR:        "192.168.1.0/24",
					Description: "Allow user custom health check port",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolTCP,
					FromPort:    30080,
					ToPort:      30080,
					CIDR:        "192.168.1.0/24",
					Description: "Allow load balancer access to port 30080",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "0.0.0.0/0",
					Description: "Default egress security group rule for IPv4",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "::/0",
					Description: "Default egress security group rule for IPv6",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv6,
				},
			},
			expectError: false,
		},
		{
			name: "Service with IP target type and pod endpoints",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "ip",
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
					},
				},
			},
			cniMode: utils.CalicoOverlay,
			resolvePodEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.1.0.1",
					Port: 8080,
					Name: "pod-1",
				},
				{
					IP:   "10.1.0.2",
					Port: 8080,
					Name: "pod-2",
				},
			},
			expectedRules: []v1alpha1.NodeSecurityGroupRule{
				{
					Protocol:    v2.SecgroupRuleProtocolTCP,
					FromPort:    8080,
					ToPort:      8080,
					CIDR:        "192.168.1.0/24",
					Description: "Allow load balancer access to port 8080",
					Direction:   v2.SecgroupRuleDirectionIngress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "0.0.0.0/0",
					Description: "Default egress security group rule for IPv4",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv4,
				},
				{
					Protocol:    v2.SecgroupRuleProtocolAll,
					FromPort:    0,
					ToPort:      65535,
					CIDR:        "::/0",
					Description: "Default egress security group rule for IPv6",
					Direction:   v2.SecgroupRuleDirectionEgress,
					EtherType:   v2.SecgroupRuleEtherTypeIPv6,
				},
			},
			expectError: false,
		},
		// TODO: Re-enable this test after implementing proper mocks for k8sRepo and vngcloudRepo
		// {
		// 	name: "Cilium native routing with multiple target ports",
		// 	service: &corev1.Service{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "test-service",
		// 			Namespace: "default",
		// 			Annotations: map[string]string{
		// 				consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
		// 			},
		// 		},
		// 		Spec: corev1.ServiceSpec{
		// 			Ports: []corev1.ServicePort{
		// 				{
		// 					Name:     "http",
		// 					Port:     80,
		// 					Protocol: corev1.ProtocolTCP,
		// 					NodePort: 30080,
		// 				},
		// 			},
		// 		},
		// 	},
		// 	cniMode: utils.CiliumNativeRouting,
		// 	resolveNodePortEndpoints: []utils.EndpointAddress{
		// 		{
		// 			IP:   "10.0.0.1",
		// 			Port: 30080,
		// 			Name: "node-1",
		// 		},
		// 	},
		// 	getListTargetPort: []int{8080, 8081},
		// 	expectedRules: []v1alpha1.NodeSecurityGroupRule{
		// 		{
		// 			Protocol: v2.SecgroupRuleProtocolTCP,
		// 			FromPort: 8080,
		// 			ToPort:   8080,
		// 			// CIDR:     "", // TODO: need to get network CIDRs or all subnet CIDRs
		// 		},
		// 		{
		// 			Protocol: v2.SecgroupRuleProtocolTCP,
		// 			FromPort: 8081,
		// 			ToPort:   8081,
		// 			// CIDR:     "", // TODO: need to get network CIDRs or all subnet CIDRs
		// 		},
		// 		{
		// 			Protocol: v2.SecgroupRuleProtocolTCP,
		// 			FromPort: 30080,
		// 			ToPort:   30080,
		// 			CIDR:     "192.168.1.0/24",
		// 		},
		// 	},
		// 	expectError: false,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockEndpointResolver := utils.NewMockEndpointResolver(t)
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
			mockNameHelper := utils.NewMockNameHelper(t)
			mockCniDetector := utils.NewMockCniDetector(t)

			// Set up mock expectations
			if tt.resolveNodePortEndpoints != nil {
				mockEndpointResolver.EXPECT().
					ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(tt.resolveNodePortEndpoints, nil)
			}

			if tt.resolvePodEndpoints != nil {
				mockEndpointResolver.EXPECT().
					ResolvePodEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
					Return(tt.resolvePodEndpoints, nil)
			}

			if tt.getListTargetPort != nil {
				mockEndpointResolver.EXPECT().
					GetListTargetPort(mock.Anything, mock.Anything, mock.Anything).
					Return(tt.getListTargetPort, nil)
			}

			// Set up CNI detector mock
			mockCniDetector.EXPECT().
				DetectCNIType(mock.Anything).
				Return(tt.cniMode, nil).
				Maybe()

			// Create the task
			task := &defaultModelBuildTask{
				service:           tt.service,
				annotationParser:  mockAnnotationParser,
				nameHelper:        mockNameHelper,
				cniDetector:       mockCniDetector,
				endpointResolver:  mockEndpointResolver,
				defaultSubnetCIDR: "192.168.1.0/24",
			}

			// Call the function
			result, err := task.buildDefaultSecurityGroupRule(context.Background(), task.defaultSubnetCIDR, map[string]string{})

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedRules, result)
			}
		})
	}
}

func TestEnsureUniqueSecgroupRules(t *testing.T) {
	task := &defaultModelBuildTask{}

	rules := []v1alpha1.NodeSecurityGroupRule{
		{
			Protocol: v2.SecgroupRuleProtocolTCP,
			FromPort: 80,
			ToPort:   80,
			CIDR:     "10.0.0.0/8",
		},
		{
			Protocol: v2.SecgroupRuleProtocolTCP,
			FromPort: 80,
			ToPort:   80,
			CIDR:     "10.0.0.0/8", // Duplicate
		},
		{
			Protocol: v2.SecgroupRuleProtocolUDP,
			FromPort: 53,
			ToPort:   53,
			CIDR:     "10.0.0.0/8",
		},
	}

	result := task.ensureUniqueSecgroupRules(rules)

	// Should have 2 unique rules (the duplicate TCP rule should be removed)
	assert.Len(t, result, 2)

	// Check that we have one TCP rule and one UDP rule
	tcpRules := 0
	udpRules := 0
	for _, rule := range result {
		if rule.Protocol == v2.SecgroupRuleProtocolTCP {
			tcpRules++
		} else if rule.Protocol == v2.SecgroupRuleProtocolUDP {
			udpRules++
		}
	}
	assert.Equal(t, 1, tcpRules)
	assert.Equal(t, 1, udpRules)
}

func TestEnsureSecgroupPING_UDP(t *testing.T) {
	task := &defaultModelBuildTask{}

	rules := []v1alpha1.NodeSecurityGroupRule{
		{
			Protocol: v2.SecgroupRuleProtocolTCP,
			FromPort: 80,
			ToPort:   80,
			CIDR:     "10.0.0.0/8",
		},
		{
			Protocol: v2.SecgroupRuleProtocolUDP,
			FromPort: 53,
			ToPort:   53,
			CIDR:     "10.0.0.0/8",
		},
	}

	result := task.ensureSecgroupPING_UDP(context.Background(), rules)

	// Should have 3 rules (original 2 + 1 ICMP rule for UDP)
	assert.Len(t, result, 3)

	// Check that we have one TCP rule, one UDP rule, and one ICMP rule
	tcpRules := 0
	udpRules := 0
	icmpRules := 0
	for _, rule := range result {
		switch rule.Protocol {
		case v2.SecgroupRuleProtocolTCP:
			tcpRules++
		case v2.SecgroupRuleProtocolUDP:
			udpRules++
		case v2.SecgroupRuleProtocolICMP:
			icmpRules++
			// Check that the ICMP rule has the correct description
			assert.Equal(t, "Allow ICMP for health check UDP port", rule.Description)
		}
	}
	assert.Equal(t, 1, tcpRules)
	assert.Equal(t, 1, udpRules)
	assert.Equal(t, 1, icmpRules)
}

func TestCoreProtocolToSecgroupProtocol(t *testing.T) {
	task := &defaultModelBuildTask{}

	// Test TCP
	result := task.coreProtocolToSecgroupProtocol(corev1.ProtocolTCP)
	assert.Equal(t, v2.SecgroupRuleProtocolTCP, result)

	// Test UDP
	result = task.coreProtocolToSecgroupProtocol(corev1.ProtocolUDP)
	assert.Equal(t, v2.SecgroupRuleProtocolUDP, result)

	// Test unsupported protocol (should default to TCP)
	result = task.coreProtocolToSecgroupProtocol(corev1.ProtocolSCTP)
	assert.Equal(t, v2.SecgroupRuleProtocolTCP, result)
}

func TestBuildIsAutoCreateSecGroup(t *testing.T) {
	tests := []struct {
		name             string
		service          *corev1.Service
		expectedIsAuto   bool
		expectedSecGroup []string
	}{
		{
			name: "No security group annotation - should auto create",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedIsAuto:   true,
			expectedSecGroup: nil,
		},
		{
			name: "Empty security group annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixSecurityGroups: "",
					},
				},
			},
			expectedIsAuto:   false,
			expectedSecGroup: []string{},
		},
		{
			name: "Single security group annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixSecurityGroups: "sg-12345",
					},
				},
			},
			expectedIsAuto:   false,
			expectedSecGroup: []string{"sg-12345"},
		},
		{
			name: "Multiple security group annotations",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixSecurityGroups: "sg-12345,sg-67890",
					},
				},
			},
			expectedIsAuto:   false,
			expectedSecGroup: []string{"sg-12345", "sg-67890"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			isAuto, secGroups := task.buildIsAutoCreateSecGroup(context.Background())

			// Assert
			assert.Equal(t, tt.expectedIsAuto, isAuto)
			assert.Equal(t, tt.expectedSecGroup, secGroups)
		})
	}
}

func TestBuildDefaultSecurityGroupRule_ErrorCases(t *testing.T) {
	// Test case for error when ResolveNodePortEndpoints fails
	t.Run("Error when ResolveNodePortEndpoints fails", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
		mockNameHelper := utils.NewMockNameHelper(t)
		mockCniDetector := utils.NewMockCniDetector(t)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))

		mockCniDetector.EXPECT().
			DetectCNIType(mock.Anything).
			Return(utils.CalicoOverlay, nil).
			Maybe()

		// Create service with instance target type
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Port:     80,
						Protocol: corev1.ProtocolTCP,
						NodePort: 30080,
					},
				},
			},
		}

		// Create the task
		task := &defaultModelBuildTask{
			service:           service,
			annotationParser:  mockAnnotationParser,
			nameHelper:        mockNameHelper,
			cniDetector:       mockCniDetector,
			endpointResolver:  mockEndpointResolver,
			defaultSubnetCIDR: "192.168.1.0/24",
		}

		// Call the function
		result, err := task.buildDefaultSecurityGroupRule(context.Background(), task.defaultSubnetCIDR, map[string]string{})

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Test case for error when ResolvePodEndpoints fails
	t.Run("Error when ResolvePodEndpoints fails", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
		mockNameHelper := utils.NewMockNameHelper(t)
		mockCniDetector := utils.NewMockCniDetector(t)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolvePodEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))

		mockCniDetector.EXPECT().
			DetectCNIType(mock.Anything).
			Return(utils.CalicoOverlay, nil).
			Maybe()

		// Create service with IP target type
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "ip",
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
				},
			},
		}

		// Create the task
		task := &defaultModelBuildTask{
			service:           service,
			annotationParser:  mockAnnotationParser,
			nameHelper:        mockNameHelper,
			cniDetector:       mockCniDetector,
			endpointResolver:  mockEndpointResolver,
			defaultSubnetCIDR: "192.168.1.0/24",
		}

		// Call the function
		result, err := task.buildDefaultSecurityGroupRule(context.Background(), task.defaultSubnetCIDR, map[string]string{})

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	// Test case for error when GetListTargetPort fails in Cilium native routing
	t.Run("Error when GetListTargetPort fails in Cilium native routing", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
		mockNameHelper := utils.NewMockNameHelper(t)
		mockCniDetector := utils.NewMockCniDetector(t)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return([]utils.EndpointAddress{
				{
					IP:   "10.0.0.1",
					Port: 30080,
					Name: "node-1",
				},
			}, nil)

		mockEndpointResolver.EXPECT().
			GetListTargetPort(mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("failed to get target ports"))

		mockCniDetector.EXPECT().
			DetectCNIType(mock.Anything).
			Return(utils.CiliumNativeRouting, nil)

		// Create service with instance target type and Cilium native routing
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     "http",
						Port:     80,
						Protocol: corev1.ProtocolTCP,
						NodePort: 30080,
					},
				},
			},
		}

		// Create the task
		task := &defaultModelBuildTask{
			service:           service,
			annotationParser:  mockAnnotationParser,
			nameHelper:        mockNameHelper,
			cniDetector:       mockCniDetector,
			endpointResolver:  mockEndpointResolver,
			defaultSubnetCIDR: "192.168.1.0/24",
		}

		// Call the function
		result, err := task.buildDefaultSecurityGroupRule(context.Background(), task.defaultSubnetCIDR, map[string]string{})

		// Assert
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}
