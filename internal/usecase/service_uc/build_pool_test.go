package service_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func TestBuildPoolsAndListeners(t *testing.T) {
	tests := []struct {
		name                     string
		service                  *corev1.Service
		resolveNodePortEndpoints []utils.EndpointAddress
		resolvePodEndpoints      []utils.EndpointAddress
		expectedPools            []v1alpha1.Pool
		expectedListeners        []v1alpha1.Listener
		expectError              bool
	}{
		{
			name: "Service with no ports should not create pools or listeners",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{}, // No ports
				},
			},
			expectedPools:     []v1alpha1.Pool{},
			expectedListeners: []v1alpha1.Listener{},
			expectError:       false,
		},
		{
			name: "Service with TCP port and instance target type",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "instance",
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
			expectedPools: []v1alpha1.Pool{
				{
					Name:     "test-pool-name",
					Protocol: loadbalancerv2.PoolProtocol(corev1.ProtocolTCP),
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.0.0.1",
							Port:        30080,
							MonitorPort: ptr.To(30080),
							Name:        "node-1",
						},
						{
							IP:          "10.0.0.2",
							Port:        30080,
							MonitorPort: ptr.To(30080),
							Name:        "node-2",
						},
					},
				},
			},
			expectedListeners: []v1alpha1.Listener{
				{
					Name:         "test-listener-name",
					Protocol:     loadbalancerv2.ListenerProtocol(corev1.ProtocolTCP),
					ProtocolPort: 80,
				},
			},
			expectError: false,
		},
		{
			name: "Service with UDP port and IP target type",
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
							Name:       "dns",
							Port:       53,
							Protocol:   corev1.ProtocolUDP,
							TargetPort: intstr.FromInt(5353),
						},
					},
				},
			},
			resolvePodEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.1.0.1",
					Port: 5353,
					Name: "pod-1",
				},
				{
					IP:   "10.1.0.2",
					Port: 5353,
					Name: "pod-2",
				},
			},
			expectedPools: []v1alpha1.Pool{
				{
					Name:     "test-pool-name",
					Protocol: loadbalancerv2.PoolProtocol(corev1.ProtocolUDP),
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.1.0.1",
							Port:        5353,
							MonitorPort: ptr.To(5353),
							Name:        "pod-1",
						},
						{
							IP:          "10.1.0.2",
							Port:        5353,
							MonitorPort: ptr.To(5353),
							Name:        "pod-2",
						},
					},
				},
			},
			expectedListeners: []v1alpha1.Listener{
				{
					Name:         "test-listener-name",
					Protocol:     loadbalancerv2.ListenerProtocol(corev1.ProtocolUDP),
					ProtocolPort: 53,
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
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:       "instance",
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
			resolveNodePortEndpoints: []utils.EndpointAddress{
				{
					IP:   "10.0.0.1",
					Port: 30080,
					Name: "node-1",
				},
			},
			expectedPools: []v1alpha1.Pool{
				{
					Name:     "test-pool-name",
					Protocol: loadbalancerv2.PoolProtocol(corev1.ProtocolTCP),
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.0.0.1",
							Port:        30080,
							MonitorPort: ptr.To(8080), // Should use health check port
							Name:        "node-1",
						},
					},
				},
			},
			expectedListeners: []v1alpha1.Listener{
				{
					Name:         "test-listener-name",
					Protocol:     loadbalancerv2.ListenerProtocol(corev1.ProtocolTCP),
					ProtocolPort: 80,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockEndpointResolver := utils.NewMockEndpointResolver(t)
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
			mockNameHelper := utils.NewMockNameHelper(t)

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

			// Set up name helper expectations
			// TODO: use real naming logic or parameterized names
			mockNameHelper.EXPECT().GenL4PoolName(mock.Anything, mock.Anything).Return("test-pool-name").Maybe()
			mockNameHelper.EXPECT().GenL4ListenerName(mock.Anything).Return("test-listener-name").Maybe()

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
				nameHelper:       mockNameHelper,
				endpointResolver: mockEndpointResolver,
			}

			// Call the function
			err := task.buildPoolsAndListeners(context.Background())

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Check pools
				assert.Equal(t, len(tt.expectedPools), len(task.vlbConfig.Spec.Pools))
				for i, expectedPool := range tt.expectedPools {
					actualPool := task.vlbConfig.Spec.Pools[i]
					assert.Equal(t, expectedPool.Name, actualPool.Name)
					assert.Equal(t, expectedPool.Protocol, actualPool.Protocol)
					assert.ElementsMatch(t, expectedPool.Members, actualPool.Members)
				}

				// Check listeners
				assert.Equal(t, len(tt.expectedListeners), len(task.vlbConfig.Spec.Listeners))
				for i, expectedListener := range tt.expectedListeners {
					actualListener := task.vlbConfig.Spec.Listeners[i]
					assert.Equal(t, expectedListener.Name, actualListener.Name)
					assert.Equal(t, expectedListener.Protocol, actualListener.Protocol)
					assert.Equal(t, expectedListener.ProtocolPort, actualListener.ProtocolPort)
				}
			}
		})
	}
}

func TestBuildPoolsAndListeners_ErrorCases(t *testing.T) {
	// Test case for error when ResolveNodePortEndpoints fails
	t.Run("Error when ResolveNodePortEndpoints fails", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
		mockNameHelper := utils.NewMockNameHelper(t)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))
		
		// Set up name helper expectations
		mockNameHelper.EXPECT().GenL4PoolName(mock.Anything, mock.Anything).Return("test-pool").Maybe()
		mockNameHelper.EXPECT().GenL4ListenerName(mock.Anything).Return("test-listener").Maybe()

		// Create service with instance target type
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "default",
				Annotations: map[string]string{
					consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "instance",
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

		// Create VLBC config
		vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
				TargetNodeLabels: map[string]string{},
			},
		}

		// Create the task
		task := &defaultModelBuildTask{
			service:          service,
			vlbConfig:        vlbc,
			annotationParser: mockAnnotationParser,
			nameHelper:       mockNameHelper,
			endpointResolver: mockEndpointResolver,
		}

		// Call the function
		err := task.buildPoolsAndListeners(context.Background())

		// Assert
		assert.Error(t, err)
	})

	// Test case for error when ResolvePodEndpoints fails
	t.Run("Error when ResolvePodEndpoints fails", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)
		mockNameHelper := utils.NewMockNameHelper(t)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolvePodEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))
		
		// Set up name helper expectations
		mockNameHelper.EXPECT().GenL4PoolName(mock.Anything, mock.Anything).Return("test-pool").Maybe()
		mockNameHelper.EXPECT().GenL4ListenerName(mock.Anything).Return("test-listener").Maybe()

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

		// Create VLBC config
		vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
			Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
				TargetNodeLabels: map[string]string{},
			},
		}

		// Create the task
		task := &defaultModelBuildTask{
			service:          service,
			vlbConfig:        vlbc,
			annotationParser: mockAnnotationParser,
			nameHelper:       mockNameHelper,
			endpointResolver: mockEndpointResolver,
		}

		// Call the function
		err := task.buildPoolsAndListeners(context.Background())

		// Assert
		assert.Error(t, err)
	})
}

func TestGetTargetType(t *testing.T) {
	tests := []struct {
		name           string
		service        *corev1.Service
		expectedTarget string
	}{
		{
			name: "No target type annotation - should default to instance",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedTarget: "instance",
		},
		{
			name: "Instance target type annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "instance",
					},
				},
			},
			expectedTarget: "instance",
		},
		{
			name: "IP target type annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "ip",
					},
				},
			},
			expectedTarget: "ip",
		},
		{
			name: "Invalid target type annotation - should default to instance",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: "invalid",
					},
				},
			},
			expectedTarget: "instance",
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
			result := task.getTargetType(context.Background())

			// Assert
			assert.Equal(t, tt.expectedTarget, result)
		})
	}
}

func TestBuildHealthcheckPort(t *testing.T) {
	tests := []struct {
		name              string
		service           *corev1.Service
		expectedPort      *int
		expectParseError  bool
	}{
		{
			name: "No health check port annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedPort: nil,
		},
		{
			name: "Valid health check port annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixHealthcheckPort: "8080",
					},
				},
			},
			expectedPort: ptr.To(8080),
		},
		{
			name: "Invalid health check port annotation (non-numeric)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixHealthcheckPort: "invalid",
					},
				},
			},
			expectedPort: nil,
		},
		{
			name: "Invalid health check port annotation (out of range - negative)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixHealthcheckPort: "-1",
					},
				},
			},
			expectedPort: nil,
		},
		{
			name: "Invalid health check port annotation (out of range - too large)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixHealthcheckPort: "65536",
					},
				},
			},
			expectedPort: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildHealthcheckPort(context.Background())

			// Assert
			assert.Equal(t, tt.expectedPort, result)
		})
	}
}

func TestBuildPoolAlgorithm(t *testing.T) {
	tests := []struct {
		name                 string
		service              *corev1.Service
		expectedAlgorithm    *loadbalancerv2.PoolAlgorithm
	}{
		{
			name: "No pool algorithm annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedAlgorithm: nil,
		},
		{
			name: "Valid round robin pool algorithm annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixPoolAlgorithm: string(loadbalancerv2.PoolAlgorithmRoundRobin),
					},
				},
			},
			expectedAlgorithm: ptr.To(loadbalancerv2.PoolAlgorithmRoundRobin),
		},
		{
			name: "Valid least connections pool algorithm annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixPoolAlgorithm: string(loadbalancerv2.PoolAlgorithmLeastConn),
					},
				},
			},
			expectedAlgorithm: ptr.To(loadbalancerv2.PoolAlgorithmLeastConn),
		},
		{
			name: "Valid source IP pool algorithm annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixPoolAlgorithm: string(loadbalancerv2.PoolAlgorithmSourceIP),
					},
				},
			},
			expectedAlgorithm: ptr.To(loadbalancerv2.PoolAlgorithmSourceIP),
		},
		{
			name: "Invalid pool algorithm annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixPoolAlgorithm: "invalid",
					},
				},
			},
			expectedAlgorithm: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildPoolAlgorithm(context.Background())

			// Assert
			assert.Equal(t, tt.expectedAlgorithm, result)
		})
	}
}

func TestBuildIdleTimeoutClient(t *testing.T) {
	tests := []struct {
		name              string
		service           *corev1.Service
		expectedTimeout   *int32
	}{
		{
			name: "No idle timeout client annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedTimeout: nil,
		},
		{
			name: "Valid idle timeout client annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutClient: "300",
					},
				},
			},
			expectedTimeout: ptr.To(int32(300)),
		},
		{
			name: "Invalid idle timeout client annotation (non-numeric)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutClient: "invalid",
					},
				},
			},
			expectedTimeout: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildIdleTimeoutClient(context.Background())

			// Assert
			assert.Equal(t, tt.expectedTimeout, result)
		})
	}
}

func TestBuildIdleTimeoutMember(t *testing.T) {
	tests := []struct {
		name              string
		service           *corev1.Service
		expectedTimeout   *int32
	}{
		{
			name: "No idle timeout member annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedTimeout: nil,
		},
		{
			name: "Valid idle timeout member annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutMember: "300",
					},
				},
			},
			expectedTimeout: ptr.To(int32(300)),
		},
		{
			name: "Invalid idle timeout member annotation (non-numeric)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutMember: "invalid",
					},
				},
			},
			expectedTimeout: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildIdleTimeoutMember(context.Background())

			// Assert
			assert.Equal(t, tt.expectedTimeout, result)
		})
	}
}

func TestBuildIdleTimeoutConnection(t *testing.T) {
	tests := []struct {
		name              string
		service           *corev1.Service
		expectedTimeout   *int32
	}{
		{
			name: "No idle timeout connection annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedTimeout: nil,
		},
		{
			name: "Valid idle timeout connection annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutConnection: "300",
					},
				},
			},
			expectedTimeout: ptr.To(int32(300)),
		},
		{
			name: "Invalid idle timeout connection annotation (non-numeric)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixIdleTimeoutConnection: "invalid",
					},
				},
			},
			expectedTimeout: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildIdleTimeoutConnection(context.Background())

			// Assert
			assert.Equal(t, tt.expectedTimeout, result)
		})
	}
}

func TestBuildInboundCIDRs(t *testing.T) {
	tests := []struct {
		name           string
		service        *corev1.Service
		expectedCIDRs  *string
	}{
		{
			name: "No inbound CIDRs annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedCIDRs: nil,
		},
		{
			name: "Single inbound CIDR annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixInboundCIDRs: "192.168.1.0/24",
					},
				},
			},
			expectedCIDRs: ptr.To("192.168.1.0/24"),
		},
		{
			name: "Multiple inbound CIDRs annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixInboundCIDRs: "192.168.1.0/24,10.0.0.0/8",
					},
				},
			},
			expectedCIDRs: ptr.To("192.168.1.0/24,10.0.0.0/8"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create VLBC config
			vlbc := &v1alpha1.VngcloudLoadBalancerConfig{
				Spec: v1alpha1.VngcloudLoadBalancerConfigSpec{
					TargetNodeLabels: map[string]string{},
				},
			}

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				vlbConfig:        vlbc,
				annotationParser: mockAnnotationParser,
			}

			// Call the function
			result := task.buildInboundCIDRs(context.Background())

			// Assert
			assert.Equal(t, tt.expectedCIDRs, result)
		})
	}
}
