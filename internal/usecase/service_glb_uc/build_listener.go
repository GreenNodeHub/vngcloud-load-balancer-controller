package service_glb_uc

import (
	"fmt"

	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
)

// genListenerName returns a deterministic listener name based on port number.
func (t *defaultModelBuildTask) genListenerName(port corev1.ServicePort) string {
	return fmt.Sprintf("listener-%d", port.Port)
}

// getListenerProtocol returns the GLB listener protocol (GLB only supports TCP).
func (t *defaultModelBuildTask) getListenerProtocol(_ corev1.Protocol) global.GlobalListenerProtocol {
	return global.GlobalListenerProtocolTCP
}
