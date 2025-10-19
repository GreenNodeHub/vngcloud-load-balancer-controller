package utils

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anngdinh/operator-helper/contexts"
	nwv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TrimString(str string, length int) string {
	return str[:MinInt(len(str), length)]
}

// HashString hash a string to a string have 10 char
func HashString(str string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(str)))
}

func StringListToString(s []string) string {
	return strings.Join(s, ",")
}

func PointerOf[T any](t T) *T {
	return &t
}

// namespacedName returns the namespaced name for k8s objects
func NamespacedName(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// serviceBackendToIntOrString converts a ServiceBackendPort (Ingress) to an IntOrString
func serviceBackendToIntOrString(port nwv1.ServiceBackendPort) intstr.IntOrString {
	if port.Name != "" {
		return intstr.FromString(port.Name)
	}
	return intstr.FromInt(int(port.Number))
}

func mergeStringArray(ctx context.Context, current, remove, add []string) ([]string, bool) {
	logger := contexts.NewContext(ctx).Log()
	logger.Debugf("  - current: %v", current)
	logger.Debugf("  - remove:  %v", remove)
	logger.Debugf("  - add:     %v", add)

	mapCurrent := make(map[string]bool)
	for _, c := range current {
		mapCurrent[c] = true
	}
	for _, r := range remove {
		delete(mapCurrent, r)
	}
	for _, a := range add {
		mapCurrent[a] = true
	}
	ret := make([]string, 0)
	for k := range mapCurrent {
		ret = append(ret, k)
	}
	if len(ret) != len(current) {
		return ret, true
	}
	for _, c := range current {
		if !mapCurrent[c] {
			return ret, true
		}
	}
	return ret, false
}

func comparePointer[T comparable](current, new *T) bool {
	if current == nil && new == nil {
		return true
	}
	if current == nil || new == nil {
		return false
	}
	return *current == *new
}

func pointerToString[T any](p *T) string {
	if p == nil {
		return "(nil)"
	}
	return fmt.Sprintf("&(%v)", *p)
}
