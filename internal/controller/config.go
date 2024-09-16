package controller

// Config struct contains ingress controller configuration
type Config struct {
	ChartVersion string `mapstructure:"chartVersion"`
	Cluster      struct {
		ClusterName string `mapstructure:"clusterName"`
		ClusterID   string `mapstructure:"clusterID"`
	} `mapstructure:"cluster"`
}
