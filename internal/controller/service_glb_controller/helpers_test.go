package service_glb_controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// newServiceWithGLBAnnotation creates a NodePort Service with the GLB enable annotation.
func newServiceWithGLBAnnotation(name, namespace string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"glb.vks.vngcloud.vn/enable": "true",
			},
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

// findGLBCByServiceOwnerLabels lists GlobalLoadBalancerConfigs using owner label selector matching the Service.
// GLBC names are generated with generateName so we look them up by labels.
func findGLBCByServiceOwnerLabels(ctx context.Context, k8sClient client.Client, svc *corev1.Service) (*v1alpha1.GlobalLoadBalancerConfig, error) {
	glbcList := &v1alpha1.GlobalLoadBalancerConfigList{}
	err := k8sClient.List(ctx, glbcList,
		client.InNamespace(svc.Namespace),
		client.MatchingLabels{
			domain.LabelOwnerResourceName: svc.Name,
			domain.LabelOwnerResourceKind: domain.KindService,
			domain.LabelOwnerResourceUid:  string(svc.UID),
		},
	)
	if err != nil {
		return nil, err
	}
	if len(glbcList.Items) == 0 {
		return nil, nil
	}
	if len(glbcList.Items) > 1 {
		return nil, fmt.Errorf("found multiple GLBCs (%d) for Service %s/%s", len(glbcList.Items), svc.Namespace, svc.Name)
	}
	return &glbcList.Items[0], nil
}
