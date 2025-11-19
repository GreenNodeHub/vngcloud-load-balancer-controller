package utils

import (
	"context"

	"github.com/anngdinh/operator-helper/contexts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CNIType represents the CNI type detected in the cluster.
type CNIType string

const (
	CalicoOverlay       CNIType = "Calico Overlay"
	CiliumOverlay       CNIType = "Cilium Overlay"
	CiliumNativeRouting CNIType = "Cilium Native Routing"
	UnknownCNI          CNIType = "Unknown CNI"
)

type CniDetector interface {
	DetectCNIType(ctx context.Context) (CNIType, error)
}

type detector struct {
	client.Client

	result *CNIType
}

// NewDetector creates a new instance of the CNI detector.
func NewDetector(kubeClient client.Client) CniDetector {
	return &detector{
		Client: kubeClient,
		result: nil,
	}
}

// DetectCNIType detects the CNI type in the cluster.
func (d *detector) DetectCNIType(ctx context.Context) (CNIType, error) {
	if d.result != nil {
		return *d.result, nil
	}

	// Check for Calico
	if d.isCalicoOverlay(ctx) {
		d.result = ptr.To(CalicoOverlay)
		return CalicoOverlay, nil
	}

	// Check for Cilium
	if d.isCiliumNativeRouting(ctx) {
		d.result = ptr.To(CiliumNativeRouting)
		return CiliumNativeRouting, nil
	}

	if d.isCiliumOverlay(ctx) {
		d.result = ptr.To(CiliumOverlay)
		return CiliumOverlay, nil
	}

	// Unknown CNI
	d.result = ptr.To(UnknownCNI)
	return UnknownCNI, nil
}

// Check if Calico Overlay is running
func (d *detector) isCalicoOverlay(ctx context.Context) bool {
	logger := contexts.NewContext(ctx).Log()

	calicoNodeDaemonSet := &appsv1.DaemonSet{}
	err := d.Client.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "calico-node"}, calicoNodeDaemonSet)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Warnf("Failed to get calico-node daemonset: %v", err)
		}
		return false
	}
	return true
}

// Check if Cilium Overlay is running
func (d *detector) isCiliumOverlay(ctx context.Context) bool {
	logger := contexts.NewContext(ctx).Log()

	ciliumDaemonSet := &appsv1.DaemonSet{}
	err := d.Client.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "cilium"}, ciliumDaemonSet)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Warnf("Failed to get cilium daemonset: %v", err)
		}
		return false
	}
	return true
}

// Check if Cilium Native Routing is running
func (d *detector) isCiliumNativeRouting(ctx context.Context) bool {
	if !d.isCiliumOverlay(ctx) {
		return false
	}

	logger := contexts.NewContext(ctx).Log()

	// get cilium-config config map
	ciliumConfigMap := &corev1.ConfigMap{}
	err := d.Client.Get(ctx, client.ObjectKey{Namespace: "kube-system", Name: "cilium-config"}, ciliumConfigMap)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Warnf("Failed to get cilium-config configmap: %v", err)
		}
		return false
	}

	// check if cilium-config have routing-mode: native
	if ciliumConfigMap.Data["routing-mode"] == "native" {
		return true
	}

	return false
}
