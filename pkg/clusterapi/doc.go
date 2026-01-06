// Package clusterapi provides utilities for interacting with Cluster API resources.
//
// This package helps retrieve kubeconfig and rest.Config for target clusters
// managed by Cluster API. It uses the official cluster-api utilities from
// sigs.k8s.io/cluster-api/util/kubeconfig to fetch kubeconfig from secrets.
//
// Usage:
//
//	// Create a client for the management cluster
//	mgmtClient, err := client.New(restConfig, client.Options{Scheme: scheme})
//	if err != nil {
//	    return err
//	}
//
//	// Create cluster API client
//	clusterAPIClient := clusterapi.NewClusterClient(mgmtClient)
//
//	// Get rest config for a target cluster
//	targetRestConfig, err := clusterAPIClient.GetRestConfig(ctx, "default", "my-cluster")
//	if err != nil {
//	    return err
//	}
//
// The kubeconfig secret is automatically fetched using the Cluster API's built-in
// kubeconfig.FromSecret function, which handles the standard naming convention
// and secret structure.
package clusterapi
