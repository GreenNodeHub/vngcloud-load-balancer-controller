package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func nodeList(names ...string) *corev1.NodeList {
	list := &corev1.NodeList{}
	for _, n := range names {
		list.Items = append(list.Items, corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: n}})
	}
	return list
}

// The node picked here decides the cluster's default network, subnet and zone, latched for the
// life of the process - so every ordering of the same cluster has to yield the same answer. A
// cached List gives no ordering guarantee, which is what made "the first node" mean a
// different node from one run to the next.
func TestFirstNodeByNameDoesNotDependOnListOrder(t *testing.T) {
	orders := [][]string{
		{"mock-node-1", "mock-node-2", "mock-node-3", "mock-node-4"},
		{"mock-node-4", "mock-node-3", "mock-node-2", "mock-node-1"},
		{"mock-node-3", "mock-node-1", "mock-node-4", "mock-node-2"},
	}

	for _, order := range orders {
		got := FirstNodeByName(nodeList(order...))
		assert.NotNil(t, got)
		assert.Equal(t, "mock-node-1", got.Name, "order %v must not change the answer", order)
	}
}

// Callers check for an empty cluster before asking, but a nil here is a nil dereference in the
// caller rather than an error they can report, so it is worth being explicit.
func TestFirstNodeByNameHandlesEmptyInput(t *testing.T) {
	assert.Nil(t, FirstNodeByName(nil))
	assert.Nil(t, FirstNodeByName(&corev1.NodeList{}))
}

// Names are not numbers: node-10 sorts before node-9, and that is fine as long as it is always
// the same one. Pinned so nobody "fixes" it into something order-dependent again.
func TestFirstNodeByNameSortsLexicographically(t *testing.T) {
	got := FirstNodeByName(nodeList("node-9", "node-10", "node-1"))
	assert.Equal(t, "node-1", got.Name)

	got = FirstNodeByName(nodeList("node-9", "node-10"))
	assert.Equal(t, "node-10", got.Name)
}
