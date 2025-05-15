package config

import (
	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
)

// Config struct contains ingress controller configuration
type Config struct {
	ChartVersion            string `mapstructure:"chartVersion"`
	MaxConcurrentReconciles int    `mapstructure:"maxConcurrentReconciles"`

	Cluster struct {
		IsRunRemote bool   `mapstructure:"isRunRemote"` // run from another cluster, watch through clusterAPI
		Namespace   string `mapstructure:"namespace"`   // if run remote, the namespace of cluster
		ClusterID   string `mapstructure:"clusterID"`   // clusterID of cluster
		Region      string `mapstructure:"region"`      // region of cluster
	} `mapstructure:"cluster"`

	Global   AuthOpts `mapstructure:"global"`
	Metadata metadata.Opts
}

type AuthOpts struct {
	IdentityURL  string `gcfg:"identity-url" mapstructure:"identityURL" name:"identity-url"`
	VServerURL   string `gcfg:"vserver-url" mapstructure:"vserverURL" name:"vserver-url"`
	ClientID     string `gcfg:"client-id" mapstructure:"clientID" name:"client-id"`
	ClientSecret string `gcfg:"client-secret" mapstructure:"clientSecret" name:"client-secret"`
	// it should help in dev mode, pass the projectID directly
	ProjectID string `gcfg:"project-id" mapstructure:"projectID" name:"project-id"`
}

func NewConfig() *Config {
	return &Config{
		Metadata: metadata.Opts{
			SearchOrder: "configDriver,metadataService",
		},
	}
}

func (c *Config) Init(setupLog logr.Logger, configFile string) error {
	// initConfig reads in config file and ENV variables if set.
	setupLog.Info("Loading configuration", "config", configFile)
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		setupLog.Error(err, "Failed to read config file")
		return err
	}

	if err := viper.Unmarshal(c); err != nil {
		setupLog.Error(err, "Unable to decode the configuration")
		return err
	}
	setupLog.Info("Configuration loaded.")
	return nil
}
