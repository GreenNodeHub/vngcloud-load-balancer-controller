package vglb_uc

import (
	"context"
	"fmt"

	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelBuildTask) buildListener(_ context.Context, port corev1.ServicePort, defaultPoolName string) *v1alpha1.GlobalListener {
	return &v1alpha1.GlobalListener{
		Name:            t.genListenerName(port),
		Protocol:        t.getListenerProtocol(port.Protocol),
		ProtocolPort:    int(port.Port),
		DefaultPoolName: &defaultPoolName,
	}
}

// Helper function for listener naming
func (t *defaultModelBuildTask) genListenerName(port corev1.ServicePort) string {
	if port.Name != "" {
		return "listener-" + port.Name
	}
	return fmt.Sprintf("listener-%s-%d", port.Protocol, port.Port)
}

// getListenerProtocol returns the GLB listener protocol
// GLB only supports TCP protocol
func (t *defaultModelBuildTask) getListenerProtocol(_ corev1.Protocol) global.GlobalListenerProtocol {
	// GLB only supports TCP
	return global.GlobalListenerProtocolTCP
}
