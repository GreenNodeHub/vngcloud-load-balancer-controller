package k8s_repo

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListNode(t *testing.T) {
	// Create a fake client with some preloaded nodes
	scheme := fake.NewClientBuilder().Build().Scheme()
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-2",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node1, node2).
		Build()

	repo := &k8sRepository{client: fakeClient}

	var nodeList corev1.NodeList
	err := repo.ListNode(context.Background(), &nodeList)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(nodeList.Items) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodeList.Items))
	}
}
