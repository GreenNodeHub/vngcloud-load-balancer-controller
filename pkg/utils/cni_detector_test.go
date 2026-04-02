package utils

import (
	"context"
	"testing"

	"fmt"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestDetectCNIType(t *testing.T) {
	t.Run("Detect Calico Overlay in kube-system", func(t *testing.T) {
		calicoDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "calico-node",
				Namespace: "kube-system",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(calicoDaemonSet).Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CalicoOverlay, cniType)
	})

	t.Run("Detect Calico Overlay in calico-system", func(t *testing.T) {
		calicoDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "calico-node",
				Namespace: "calico-system",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(calicoDaemonSet).Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CalicoOverlay, cniType)
	})

	t.Run("Detect Cilium Native Routing", func(t *testing.T) {
		// Create a fake client with Cilium DaemonSet and ConfigMap
		ciliumDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium",
				Namespace: "kube-system",
			},
		}
		ciliumConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-config",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"routing-mode": "native",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(ciliumDaemonSet, ciliumConfigMap).Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CiliumNativeRouting, cniType)
	})

	t.Run("Detect Cilium Overlay", func(t *testing.T) {
		// Create a fake client with Cilium DaemonSet without native routing
		ciliumDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium",
				Namespace: "kube-system",
			},
		}
		ciliumConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium-config",
				Namespace: "kube-system",
			},
			Data: map[string]string{
				"routing-mode": "overlay",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(ciliumDaemonSet, ciliumConfigMap).Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CiliumOverlay, cniType)
	})

	t.Run("Detect Cilium Overlay without configmap", func(t *testing.T) {
		// Cilium DaemonSet exists but no cilium-config ConfigMap
		ciliumDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium",
				Namespace: "kube-system",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(ciliumDaemonSet).Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CiliumOverlay, cniType)
	})

	t.Run("Detect Unknown CNI", func(t *testing.T) {
		// Create a fake client with no CNI-related DaemonSets or ConfigMaps
		fakeClient := fake.NewClientBuilder().Build()

		detector := NewDetector(fakeClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, UnknownCNI, cniType)
	})

	t.Run("Calico Get error falls through to unknown", func(t *testing.T) {
		// Simulate a non-NotFound error when getting calico-node daemonset
		fakeClient := fake.NewClientBuilder().Build()
		errClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
			Get: func(
				ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption,
			) error {
				if key.Name == "calico-node" {
					return fmt.Errorf("connection refused")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})

		detector := NewDetector(errClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, UnknownCNI, cniType)
	})

	t.Run("Cilium Get error falls through to unknown", func(t *testing.T) {
		// Simulate a non-NotFound error when getting cilium daemonset
		fakeClient := fake.NewClientBuilder().Build()
		errClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
			Get: func(
				ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption,
			) error {
				if key.Name == "cilium" {
					return fmt.Errorf("connection refused")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})

		detector := NewDetector(errClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, UnknownCNI, cniType)
	})

	t.Run("Cilium configmap Get error falls through to overlay", func(t *testing.T) {
		// Cilium DaemonSet exists but configmap get returns non-NotFound error
		ciliumDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cilium",
				Namespace: "kube-system",
			},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(ciliumDaemonSet).Build()
		errClient := interceptor.NewClient(fakeClient, interceptor.Funcs{
			Get: func(
				ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption,
			) error {
				if key.Name == "cilium-config" {
					return fmt.Errorf("connection refused")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})

		detector := NewDetector(errClient)
		cniType, err := detector.DetectCNIType(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, CiliumOverlay, cniType)
	})

	t.Run("Cached result is returned on second call", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().Build()

		detector := NewDetector(fakeClient)
		cniType1, err := detector.DetectCNIType(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, UnknownCNI, cniType1)

		// Second call should return cached result
		cniType2, err := detector.DetectCNIType(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, UnknownCNI, cniType2)
	})
}

func TestRealCluster(t *testing.T) {
	// configPath := "/home/annd2/Downloads/annd2-clean.txt" // -> calico overlay
	configPath := ""
	if configPath == "" {
		t.Skip("Skipping test; no kubeconfig provided")
	}

	// init new kubernetes client
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		t.Fatalf("failed to build kubeconfig: %v", err)
	}
	clientset, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		t.Fatalf("failed to create clientset: %v", err)
	}

	detector := NewDetector(clientset)
	cniType, err := detector.DetectCNIType(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	t.Logf("Detected CNI type: %s", cniType)
}
