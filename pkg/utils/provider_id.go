package utils

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"
)

// providerID
const (
	// Define the regular expression pattern
	patternPrefix = `vngcloud:\/\/`
	rawPrefix     = `vngcloud://`
	pattern       = "^" + patternPrefix + "ins-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

var (
	vngCloudProviderIDRegex = regexp.MustCompile(pattern)
)

func matchCloudProviderPattern(pproviderID string) bool {
	return vngCloudProviderIDRegex.MatchString(pproviderID)
}

func getProviderID(pnode *corev1.Node) string {
	return pnode.Spec.ProviderID[len(rawPrefix):len(pnode.Spec.ProviderID)]
}

// GetListProviderIdFromNodes returns the list of provider IDs.
func GetListProviderIdFromNodes(pnodes []*corev1.Node) []string {
	var providerIDs []string
	for _, node := range pnodes {
		if node != nil && (matchCloudProviderPattern(node.Spec.ProviderID)) {
			providerIDs = append(providerIDs, getProviderID(node))
		}
	}

	return providerIDs
}

func GetListProviderIdFromNodeList(pnodes *corev1.NodeList) []string {
	if pnodes == nil || len(pnodes.Items) == 0 {
		return nil
	}
	var providerIDs []string
	for _, node := range pnodes.Items {
		if matchCloudProviderPattern(node.Spec.ProviderID) {
			providerIDs = append(providerIDs, getProviderID(&node))
		}
	}
	return providerIDs
}
