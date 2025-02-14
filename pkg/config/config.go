package config

import (
	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils/metadata"
)

// Config struct contains ingress controller configuration
type Config struct {
	ChartVersion string `mapstructure:"chartVersion"`
	Cluster      struct {
		ClusterName string `mapstructure:"clusterName"`
		ClusterID   string `mapstructure:"clusterID"`
		Region      string `mapstructure:"region"`
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
	setupLog.Info("Configuration loaded", "config", c)
	return nil
}
