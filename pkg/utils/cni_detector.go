package utils

import (
	"context"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	DetectCNIType() (CNIType, error)
}

type detector struct {
	client.Client
}

// NewDetector creates a new instance of the CNI detector.
func NewDetector(kubeClient client.Client) CniDetector {
	return &detector{Client: kubeClient}
}

// DetectCNIType detects the CNI type in the cluster.
func (d *detector) DetectCNIType() (CNIType, error) {
	// Check for Calico
	if d.isCalicoOverlay() {
		return CalicoOverlay, nil
	}

	// Check for Cilium
	if d.isCiliumNativeRouting() {
		return CiliumNativeRouting, nil
	}

	if d.isCiliumOverlay() {
		return CiliumOverlay, nil
	}

	return UnknownCNI, nil
}

// Check if Calico Overlay is running
func (d *detector) isCalicoOverlay() bool {
	calicoNodeDaemonSet := &appsv1.DaemonSet{}
	err := d.Client.Get(context.TODO(), client.ObjectKey{Namespace: "kube-system", Name: "calico-node"}, calicoNodeDaemonSet)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.Warnf("Failed to get calico-node daemonset: %v", err)
		}
		return false
	}
	return true
}

// Check if Cilium Overlay is running
func (d *detector) isCiliumOverlay() bool {
	ciliumDaemonSet := &appsv1.DaemonSet{}
	err := d.Client.Get(context.TODO(), client.ObjectKey{Namespace: "kube-system", Name: "cilium"}, ciliumDaemonSet)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.Warnf("Failed to get cilium daemonset: %v", err)
		}
		return false
	}
	return true
}

// Check if Cilium Native Routing is running
func (d *detector) isCiliumNativeRouting() bool {
	if !d.isCiliumOverlay() {
		return false
	}

	// get cilium-config config map
	ciliumConfigMap := &corev1.ConfigMap{}
	err := d.Client.Get(context.TODO(), client.ObjectKey{Namespace: "kube-system", Name: "cilium-config"}, ciliumConfigMap)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			logrus.Warnf("Failed to get cilium-config configmap: %v", err)
		}
		return false
	}

	// check if cilium-config have routing-mode: native
	if ciliumConfigMap.Data["routing-mode"] == "native" {
		return true
	}

	return false
}
