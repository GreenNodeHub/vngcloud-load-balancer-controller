package vglb_controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// newVGLBResource creates a minimal VngcloudGlobalLoadBalancer resource for testing.
func newVGLBResource(name, namespace string) *v1alpha1.VngcloudGlobalLoadBalancer {
	return &v1alpha1.VngcloudGlobalLoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "VngcloudGlobalLoadBalancer",
			APIVersion: "vks.vngcloud.vn/v1alpha1",
		},
		Spec: v1alpha1.VngcloudGlobalLoadBalancerSpec{},
	}
}

// newNodePortService creates a NodePort Service for testing.
func newNodePortService(name, namespace string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeNodePort,
			Ports: ports,
			Selector: map[string]string{
				"app": name,
			},
		},
	}
}

// newNodeWithLabels creates a Node with VKS network labels and an internal IP.
func newNodeWithLabels(name string, internalIP string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"vks.vngcloud.vn/mgmt-zone":  "hcm03b",
				"vks.vngcloud.vn/network-id": "net-test-vpc",
				"vks.vngcloud.vn/subnet-id":  "sub-test-subnet",
			},
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: internalIP},
			},
		},
	}
}

// findGLBCByOwnerLabels lists GlobalLoadBalancerConfigs using owner label selector matching the VGLB.
// GLBC names are generated with generateName so we look them up by labels.
func findGLBCByOwnerLabels(ctx context.Context, k8sClient client.Client, vglbObj *v1alpha1.VngcloudGlobalLoadBalancer) (*v1alpha1.GlobalLoadBalancerConfig, error) {
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := k8sClient.List(ctx, glbcList,
		client.InNamespace(vglbObj.Namespace),
		client.MatchingLabels{
			domain.LabelOwnerResourceName: vglbObj.Name,
			domain.LabelOwnerResourceKind: domain.KindVngcloudGlobalLoadBalancer,
			domain.LabelOwnerResourceUid:  string(vglbObj.UID),
		},
	)
	if err != nil {
		return nil, err
	}
	if len(glbcList.Items) == 0 {
		return nil, nil
	}
	if len(glbcList.Items) > 1 {
		return nil, fmt.Errorf("found multiple GLBCs (%d) for VGLB %s/%s", len(glbcList.Items), vglbObj.Namespace, vglbObj.Name)
	}
	return &glbcList.Items[0], nil
}
