package k8s

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var vngCloudProviderIDRegex = regexp.MustCompile("^ins-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

// GetNodeCondition will get pointer to Node's existing condition.
// returns nil if no matching condition found.
func GetNodeCondition(node *corev1.Node, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	if node == nil {
		return nil
	}
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == conditionType {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

func ExtractNodeInstanceID(node *corev1.Node) (string, error) {
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return "", errors.Errorf("providerID is not specified for node: %s", node.Name)
	}

	providerIDParts := strings.Split(providerID, "/")
	instanceID := providerIDParts[len(providerIDParts)-1]
	if !vngCloudProviderIDRegex.MatchString(instanceID) {
		return "", errors.Errorf("providerID %s is invalid for VNGCLOUD instances, node: %s", providerID, node.Name)
	}
	return instanceID, nil
}

func FilterNodeWithLabel(nodes []*corev1.Node, nodeLabels map[string]string) []*corev1.Node {
	if len(nodeLabels) == 0 {
		return nodes
	}
	var filtered []*corev1.Node
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Labels == nil {
			continue
		}
		if labels.Set(nodeLabels).AsSelector().Matches(labels.Set(node.Labels)) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func FilterNodeWithoutLabel(nodes []*corev1.Node, nodeLabels map[string]string) []*corev1.Node {
	if len(nodeLabels) == 0 {
		return []*corev1.Node{}
	}
	var filtered []*corev1.Node
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Labels == nil {
			filtered = append(filtered, node)
			continue
		}
		if !labels.Set(nodeLabels).AsSelector().Matches(labels.Set(node.Labels)) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}
