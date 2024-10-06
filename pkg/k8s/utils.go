package k8s

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NamespacedName returns the namespaced name for k8s objects
func NamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// ToSliceOfNamespacedNames gets the slice of types.NamespacedName from the input slice s
func ToSliceOfNamespacedNames[T metav1.ObjectMetaAccessor](s []T) []types.NamespacedName {
	result := make([]types.NamespacedName, len(s))
	for i, v := range s {
		result[i] = NamespacedName(v.GetObjectMeta())
	}
	return result
}

// NodeSlicesEqual check if two nodes equals to each other.
func NodeSlicesEqual(x, y []*corev1.Node) bool {
	if len(x) != len(y) {
		return false
	}
	return stringSlicesEqual(NodeNames(x), NodeNames(y))
}

// NodeNames get all the node names.
func NodeNames(nodes []*corev1.Node) []string {
	ret := make([]string, len(nodes))
	for i, node := range nodes {
		ret[i] = node.Name
	}
	return ret
}

func stringSlicesEqual(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	if !sort.StringsAreSorted(x) {
		sort.Strings(x)
	}
	if !sort.StringsAreSorted(y) {
		sort.Strings(y)
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
