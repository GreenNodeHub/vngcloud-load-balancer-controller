package config

import "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"

// Config struct contains ingress controller configuration
type Config struct {
	ChartVersion string `mapstructure:"chartVersion"`
	Cluster      struct {
		ClusterName string `mapstructure:"clusterName"`
		ClusterID   string `mapstructure:"clusterID"`
	} `mapstructure:"cluster"`

	Global   AuthOpts `mapstructure:"global"`
	Metadata metadata.Opts
}

type AuthOpts struct {
	IdentityURL  string `gcfg:"identity-url" mapstructure:"identityURL" name:"identity-url"`
	VServerURL   string `gcfg:"vserver-url" mapstructure:"vserverURL" name:"vserver-url"`
	ClientID     string `gcfg:"client-id" mapstructure:"clientID" name:"client-id"`
	ClientSecret string `gcfg:"client-secret" mapstructure:"clientSecret" name:"client-secret"`
}

func NewConfig() *Config {
	return &Config{
		Metadata: metadata.Opts{
			SearchOrder: "configDriver,metadataService",
		},
	}
}
