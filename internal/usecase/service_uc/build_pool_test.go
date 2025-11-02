package service_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
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
							MonitorPort: 30080,
							Name:        "node-1",
						},
						{
							IP:          "10.0.0.2",
							Port:        30080,
							MonitorPort: 30080,
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
							MonitorPort: 5353,
							Name:        "pod-1",
						},
						{
							IP:          "10.1.0.2",
							Port:        5353,
							MonitorPort: 5353,
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
							MonitorPort: 8080, // Should use health check port
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
		{
			name: "Service with proxy protocol enabled for specific port",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:          string(domain.TargetTypeInstance),
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "http",
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
					Protocol: loadbalancerv2.PoolProtocolProxy, // Should be PROXY protocol
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.0.0.1",
							Port:        30080,
							MonitorPort: 30080,
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
		{
			name: "Service with proxy protocol enabled for all ports (wildcard)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:          string(domain.TargetTypeInstance),
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "*",
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
					Protocol: loadbalancerv2.PoolProtocolProxy, // Should be PROXY protocol
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.0.0.1",
							Port:        30080,
							MonitorPort: 30080,
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
		{
			name: "Service with proxy protocol enabled but UDP protocol (should not apply)",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:          "ip",
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "*",
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
			},
			expectedPools: []v1alpha1.Pool{
				{
					Name:     "test-pool-name",
					Protocol: loadbalancerv2.PoolProtocol(corev1.ProtocolUDP), // Should remain UDP, not PROXY
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.1.0.1",
							Port:        5353,
							MonitorPort: 5353,
							Name:        "pod-1",
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
			name: "Service with proxy protocol enabled for non-matching port name",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType:          string(domain.TargetTypeInstance),
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "https", // different from port name
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:     "http", // port name is 'http', not 'https'
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
					Protocol: loadbalancerv2.PoolProtocol(corev1.ProtocolTCP), // Should remain TCP, not PROXY
					Members: []v1alpha1.PoolMember{
						{
							IP:          "10.0.0.1",
							Port:        30080,
							MonitorPort: 30080,
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

			// Create real name helper
			realNameHelper := utils.NewNameHelper(
				"test-cluster-id",
				"service",
				tt.service.Namespace,
				tt.service.Name,
			)

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

			// Create the task
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				nameHelper:       realNameHelper,
				endpointResolver: mockEndpointResolver,
			}

			// Call the function
			pools, listeners, err := task.buildPoolsAndListeners(context.Background(), map[string]string{})

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Check pools
				assert.Equal(t, len(tt.expectedPools), len(pools))
				for i, expectedPool := range tt.expectedPools {
					actualPool := pools[i]
					assert.Equal(t, expectedPool.Protocol, actualPool.Protocol)
					assert.ElementsMatch(t, expectedPool.Members, actualPool.Members)
					// For real name helper, verify full generated pool name matches expected format
					expectedPoolName := realNameHelper.GenL4PoolName(tt.service.Spec.Ports[i], string(expectedPool.Protocol))
					assert.Equal(t, expectedPoolName, actualPool.Name)
				}

				// Check listeners
				assert.Equal(t, len(tt.expectedListeners), len(listeners))
				for i, expectedListener := range tt.expectedListeners {
					actualListener := listeners[i]
					assert.Equal(t, expectedListener.Protocol, actualListener.Protocol)
					assert.Equal(t, expectedListener.ProtocolPort, actualListener.ProtocolPort)
					// For real name helper, verify full generated listener name matches expected
					expectedListenerName := realNameHelper.GenL4ListenerName(tt.service.Spec.Ports[i])
					assert.Equal(t, expectedListenerName, actualListener.Name)
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

		// Create real name helper
		realNameHelper := utils.NewNameHelper(
			"test-cluster-id",
			"service",
			"default",
			"test-service",
		)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))

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
		logger := logrus.New().WithField("test", "build_pool")
		task := &defaultModelBuildTask{
			service:          service,
			annotationParser: mockAnnotationParser,
			nameHelper:       realNameHelper,
			endpointResolver: mockEndpointResolver,
			logger:           logger,
		}

		// Call the function
		_, _, err := task.buildPoolsAndListeners(context.Background(), map[string]string{})

		// Assert
		assert.Error(t, err)
	})

	// Test case for error when ResolvePodEndpoints fails
	t.Run("Error when ResolvePodEndpoints fails", func(t *testing.T) {
		// Create mocks
		mockEndpointResolver := utils.NewMockEndpointResolver(t)
		mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

		// Create real name helper
		realNameHelper := utils.NewNameHelper(
			"test-cluster-id",
			"service",
			"default",
			"test-service",
		)

		// Set up mock expectations
		mockEndpointResolver.EXPECT().
			ResolvePodEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, fmt.Errorf("endpoint resolution error"))

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
		logger := logrus.New().WithField("test", "build_pool")
		task := &defaultModelBuildTask{
			service:          service,
			annotationParser: mockAnnotationParser,
			nameHelper:       realNameHelper,
			endpointResolver: mockEndpointResolver,
			logger:           logger,
		}

		// Call the function
		_, _, err := task.buildPoolsAndListeners(context.Background(), map[string]string{})

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
			expectedTarget: string(domain.TargetTypeInstance),
		},
		{
			name: "Instance target type annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixTargetType: string(domain.TargetTypeInstance),
					},
				},
			},
			expectedTarget: string(domain.TargetTypeInstance),
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
			expectedTarget: string(domain.TargetTypeInstance),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name             string
		service          *corev1.Service
		expectedPort     *int
		expectParseError bool
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name              string
		service           *corev1.Service
		expectedAlgorithm *loadbalancerv2.PoolAlgorithm
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name            string
		service         *corev1.Service
		expectedTimeout *int32
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name            string
		service         *corev1.Service
		expectedTimeout *int32
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name            string
		service         *corev1.Service
		expectedTimeout *int32
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
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
		name          string
		service       *corev1.Service
		expectedCIDRs *string
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

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
			}

			// Call the function
			result := task.buildInboundCIDRs(context.Background())

			// Assert
			assert.Equal(t, tt.expectedCIDRs, result)
		})
	}
}

func TestBuildEnableProxyProtocol(t *testing.T) {
	tests := []struct {
		name                  string
		service               *corev1.Service
		expectedProxyProtocol []string
	}{
		{
			name: "No enable proxy protocol annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-service",
					Namespace:   "default",
					Annotations: map[string]string{},
				},
			},
			expectedProxyProtocol: nil,
		},
		{
			name: "Single port name in proxy protocol annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "http",
					},
				},
			},
			expectedProxyProtocol: []string{"http"},
		},
		{
			name: "Multiple port names in proxy protocol annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "http,https",
					},
				},
			},
			expectedProxyProtocol: []string{"http", "https"},
		},
		{
			name: "Wildcard proxy protocol annotation",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "*",
					},
				},
			},
			expectedProxyProtocol: []string{"*"},
		},
		{
			name: "Mix of wildcard and specific port names",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
					Annotations: map[string]string{
						consts.SERVICE_ANNOTATION_PREFIX + "/" + annotations.SuffixEnableProxyProtocol: "*,http,https",
					},
				},
			},
			expectedProxyProtocol: []string{"*", "http", "https"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			mockAnnotationParser := annotations.NewSuffixAnnotationParser(consts.SERVICE_ANNOTATION_PREFIX)

			// Create the task
			logger := logrus.New().WithField("test", "build_pool")
			task := &defaultModelBuildTask{
				service:          tt.service,
				annotationParser: mockAnnotationParser,
				logger:           logger,
			}

			// Call the function
			result := task.buildEnableProxyProtocol(context.Background())

			// Assert
			assert.Equal(t, tt.expectedProxyProtocol, result)
		})
	}
}
