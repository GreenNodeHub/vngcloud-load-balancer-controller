package clusterapi

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterClient handles interactions with Cluster API clusters
type ClusterClient struct {
	client client.Client
}

// NewClusterClient creates a new ClusterClient
func NewClusterClient(client client.Client) *ClusterClient {
	return &ClusterClient{
		client: client,
	}
}

// GetRestConfig retrieves the rest config for a target cluster by reading its kubeconfig secret
// namespace: the namespace where the Cluster resource exists
// clusterID: the name of the Cluster resource
func (c *ClusterClient) GetRestConfig(ctx context.Context, namespace, clusterID string) (*rest.Config, error) {
	clusterKey := types.NamespacedName{
		Namespace: namespace,
		Name:      clusterID,
	}

	// Use the built-in cluster-api utility to fetch kubeconfig from secret
	kubeconfigBytes, err := kubeconfig.FromSecret(ctx, c.client, clusterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig for cluster %s/%s: %w", namespace, clusterID, err)
	}

	// Parse the kubeconfig to create rest.Config
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config from kubeconfig: %w", err)
	}

	return restConfig, nil
}
