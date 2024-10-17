package builder

import (
	"crypto/sha256"
	"fmt"
	"strings"

	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
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
func namespacedName(obj metav1.Object) types.NamespacedName {
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

func coreProtocolToSecgroupProtocol(protocol corev1.Protocol) networkv2.SecgroupRuleProtocol {
	switch protocol {
	case corev1.ProtocolTCP:
		return networkv2.SecgroupRuleProtocolTCP
	case corev1.ProtocolUDP:
		return networkv2.SecgroupRuleProtocolUDP
	default:
		return networkv2.SecgroupRuleProtocolTCP
	}
}
