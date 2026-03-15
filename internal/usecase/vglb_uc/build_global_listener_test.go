package vglb_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestGenListenerName(t *testing.T) {
	tests := []struct {
		name         string
		port         corev1.ServicePort
		expectedName string
		description  string
	}{
		{
			name: "with_port_name",
			port: corev1.ServicePort{
				Name:     "http",
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			},
			expectedName: "listener-http",
			description:  "should use port name when available",
		},
		{
			name: "without_port_name",
			port: corev1.ServicePort{
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			},
			expectedName: "listener-TCP-80",
			description:  "should use protocol and port number when name is empty",
		},
		{
			name: "https_named_port",
			port: corev1.ServicePort{
				Name:     "https",
				Port:     443,
				Protocol: corev1.ProtocolTCP,
			},
			expectedName: "listener-https",
			description:  "should use https port name",
		},
		{
			name: "grpc_named_port",
			port: corev1.ServicePort{
				Name:     "grpc",
				Port:     9090,
				Protocol: corev1.ProtocolTCP,
			},
			expectedName: "listener-grpc",
			description:  "should use grpc port name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}

			result := task.genListenerName(tt.port)

			assert.Equal(t, tt.expectedName, result, tt.description)
		})
	}
}

func TestGetListenerProtocol(t *testing.T) {
	task := &defaultModelBuildTask{}

	// Test with TCP
	result := task.getListenerProtocol(corev1.ProtocolTCP)
	assert.Equal(t, global.GlobalListenerProtocolTCP, result, "should return TCP for GLB listener")

	// Test with UDP (should still return TCP as GLB only supports TCP)
	result = task.getListenerProtocol(corev1.ProtocolUDP)
	assert.Equal(t, global.GlobalListenerProtocolTCP, result, "should return TCP even for UDP input")
}

func TestBuildListener(t *testing.T) {
	tests := []struct {
		name            string
		port            corev1.ServicePort
		defaultPoolName string
		expectedName    string
		expectedPort    int
		description     string
	}{
		{
			name: "http_listener",
			port: corev1.ServicePort{
				Name:     "http",
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			},
			defaultPoolName: "pool-http",
			expectedName:    "listener-http",
			expectedPort:    80,
			description:     "should create HTTP listener correctly",
		},
		{
			name: "https_listener",
			port: corev1.ServicePort{
				Name:     "https",
				Port:     443,
				Protocol: corev1.ProtocolTCP,
			},
			defaultPoolName: "pool-https",
			expectedName:    "listener-https",
			expectedPort:    443,
			description:     "should create HTTPS listener correctly",
		},
		{
			name: "unnamed_listener",
			port: corev1.ServicePort{
				Port:     8080,
				Protocol: corev1.ProtocolTCP,
			},
			defaultPoolName: "pool-TCP-8080",
			expectedName:    "listener-TCP-8080",
			expectedPort:    8080,
			description:     "should create unnamed listener correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}

			result := task.buildListener(context.Background(), tt.port, tt.defaultPoolName)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedName, result.Name, tt.description)
			assert.Equal(t, tt.expectedPort, result.ProtocolPort, tt.description)
			assert.Equal(t, global.GlobalListenerProtocolTCP, result.Protocol)
			assert.NotNil(t, result.DefaultPoolName)
			assert.Equal(t, tt.defaultPoolName, *result.DefaultPoolName)
		})
	}
}
