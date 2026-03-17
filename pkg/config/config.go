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

	LoadBalancerOpts       LoadBalancerOpts       `mapstructure:"loadBalancerOpts"`
	GlobalLoadBalancerOpts GlobalLoadBalancerOpts `mapstructure:"globalLoadBalancerOpts"`

	Global   AuthOpts `mapstructure:"global"`
	Metadata metadata.Opts
}

type AuthOpts struct {
	IdentityURL  string `gcfg:"identity-url" mapstructure:"identityURL" name:"identity-url"`
	VServerURL   string `gcfg:"vserver-url" mapstructure:"vserverURL" name:"vserver-url"`
	ClientID     string `gcfg:"client-id" mapstructure:"clientID" name:"client-id"`
	ClientSecret string `gcfg:"client-secret" mapstructure:"clientSecret" name:"client-secret"`
	// it should help in dev mode, pass the projectID and userID directly
	ProjectID string `gcfg:"project-id" mapstructure:"projectID" name:"project-id"`
	UserID    int    `gcfg:"user-id" mapstructure:"userID" name:"user-id"`

	// for super client to manage INTERVPC load balancer (optional)
	SuperClientID     string `gcfg:"super-client-id" mapstructure:"superClientID" name:"super-client-id"`
	SuperClientSecret string `gcfg:"super-client-secret" mapstructure:"superClientSecret" name:"super-client-secret"`
}

type LoadBalancerOpts struct {
	DefaultL4PackageName string `gcfg:"default-l4-package-name" mapstructure:"defaultL4PackageName" name:"default-l4-package-name"`
	DefaultL7PackageName string `gcfg:"default-l7-package-name" mapstructure:"defaultL7PackageName" name:"default-l7-package-name"`
	DefaultScheme        string `gcfg:"default-scheme" mapstructure:"defaultScheme" name:"default-scheme"`

	// Pool defaults
	DefaultPoolAlgorithm      string `gcfg:"default-pool-algorithm" mapstructure:"defaultPoolAlgorithm" name:"default-pool-algorithm"`
	DefaultHealthyThreshold   int    `gcfg:"default-healthy-threshold" mapstructure:"defaultHealthyThreshold" name:"default-healthy-threshold"`
	DefaultUnhealthyThreshold int    `gcfg:"default-unhealthy-threshold" mapstructure:"defaultUnhealthyThreshold" name:"default-unhealthy-threshold"`
	DefaultInterval           int    `gcfg:"default-interval" mapstructure:"defaultInterval" name:"default-interval"`
	DefaultTimeout            int    `gcfg:"default-timeout" mapstructure:"defaultTimeout" name:"default-timeout"`

	// Listener defaults
	DefaultTimeoutClient     int    `gcfg:"default-timeout-client" mapstructure:"defaultTimeoutClient" name:"default-timeout-client"`
	DefaultTimeoutMember     int    `gcfg:"default-timeout-member" mapstructure:"defaultTimeoutMember" name:"default-timeout-member"`
	DefaultTimeoutConnection int    `gcfg:"default-timeout-connection" mapstructure:"defaultTimeoutConnection" name:"default-timeout-connection"`
	DefaultAllowedCidrs      string `gcfg:"default-allowed-cidrs" mapstructure:"defaultAllowedCidrs" name:"default-allowed-cidrs"`
}

type GlobalLoadBalancerOpts struct {
	DefaultL4PackageName string `gcfg:"default-l4-package-name" mapstructure:"defaultL4PackageName" name:"default-l4-package-name"`

	// Pool defaults
	DefaultPoolAlgorithm      string `gcfg:"default-pool-algorithm" mapstructure:"defaultPoolAlgorithm" name:"default-pool-algorithm"`
	DefaultHealthyThreshold   int    `gcfg:"default-healthy-threshold" mapstructure:"defaultHealthyThreshold" name:"default-healthy-threshold"`
	DefaultUnhealthyThreshold int    `gcfg:"default-unhealthy-threshold" mapstructure:"defaultUnhealthyThreshold" name:"default-unhealthy-threshold"`
	DefaultInterval           int    `gcfg:"default-interval" mapstructure:"defaultInterval" name:"default-interval"`
	DefaultTimeout            int    `gcfg:"default-timeout" mapstructure:"defaultTimeout" name:"default-timeout"`

	// Listener defaults
	DefaultTimeoutClient     int    `gcfg:"default-timeout-client" mapstructure:"defaultTimeoutClient" name:"default-timeout-client"`
	DefaultTimeoutMember     int    `gcfg:"default-timeout-member" mapstructure:"defaultTimeoutMember" name:"default-timeout-member"`
	DefaultTimeoutConnection int    `gcfg:"default-timeout-connection" mapstructure:"defaultTimeoutConnection" name:"default-timeout-connection"`
	DefaultAllowedCidrs      string `gcfg:"default-allowed-cidrs" mapstructure:"defaultAllowedCidrs" name:"default-allowed-cidrs"`
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
	return nil
}
