package config

import (
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
)

// Mock logger using logr's funcr package
func newMockLogger() logr.Logger {
	return funcr.New(
		func(prefix, args string) {
			// You can log the output somewhere if needed for test validation
		}, funcr.Options{},
	)
}

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	// Test default Metadata.SearchOrder value
	expectedSearchOrder := "configDriver,metadataService"
	if config.Metadata.SearchOrder != expectedSearchOrder {
		t.Errorf("NewConfig() Metadata.SearchOrder = %s; want %s", config.Metadata.SearchOrder, expectedSearchOrder)
	}
}

func TestConfig_Init(t *testing.T) {
	setupLog := newMockLogger()

	// Create a temporary config file for testing
	tmpFile, err := os.CreateTemp("", "sfsafdfsa.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }() // Clean up after test

	// Write some content to the config file
	configContent := `
chartVersion: "1.0.0"
cluster:
  namespace: "test-namespace"
  region: "hcm"
  clusterID: "test-cluster-id"
global:
  identityURL: "https://identity.url"
  vserverURL: "https://vserver.url"
  clientID: "test-client-id"
  clientSecret: "test-client-secret"
  projectID: "test-project-id"
`
	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}

	// Test with valid config file
	config := NewConfig()
	err = config.Init(setupLog, tmpFile.Name())
	assert.NoError(t, err, "Init should succeed with valid config file")

	// Verify that the configuration was loaded correctly
	assert.Equal(t, "1.0.0", config.ChartVersion)
	assert.Equal(t, "test-namespace", config.Cluster.Namespace)
	assert.Equal(t, "test-cluster-id", config.Cluster.ClusterID)
	assert.Equal(t, "hcm", config.Cluster.Region)
	assert.Equal(t, false, config.Cluster.IsRunRemote)
	assert.Equal(t, "https://identity.url", config.Global.IdentityURL)
	assert.Equal(t, "https://vserver.url", config.Global.VServerURL)
	assert.Equal(t, "test-client-id", config.Global.ClientID)
	assert.Equal(t, "test-client-secret", config.Global.ClientSecret)
	assert.Equal(t, "test-project-id", config.Global.ProjectID)

	// Test with an invalid config file path
	err = config.Init(setupLog, "invalid-path.yaml")
	assert.Error(t, err, "Init should return error for invalid config file path")
}

func TestConfig_Init_Success(t *testing.T) {
	setupLog := newMockLogger()

	// Create a temporary config file for testing
	tmpFile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }() // Clean up after test

	// Write valid content to the config file
	configContent := `
chartVersion: "1.0.0"
cluster:
  namespace: "test-cluster"
  clusterID: "test-cluster-id"
global:
  identityURL: "https://identity.url"
  vserverURL: "https://vserver.url"
  clientID: "test-client-id"
  clientSecret: "test-client-secret"
  projectID: "test-project-id"
`
	if _, err := tmpFile.Write([]byte(configContent)); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}
	_ = tmpFile.Close()

	config := NewConfig()
	err = config.Init(setupLog, tmpFile.Name())
	assert.NoError(t, err, "Init should succeed with valid config file")

	// Check if the values were loaded correctly
	assert.Equal(t, "1.0.0", config.ChartVersion)
	assert.Equal(t, "test-cluster", config.Cluster.Namespace)
	assert.Equal(t, "test-cluster-id", config.Cluster.ClusterID)
	assert.Equal(t, "https://identity.url", config.Global.IdentityURL)
	assert.Equal(t, "https://vserver.url", config.Global.VServerURL)
	assert.Equal(t, "test-client-id", config.Global.ClientID)
	assert.Equal(t, "test-client-secret", config.Global.ClientSecret)
	assert.Equal(t, "test-project-id", config.Global.ProjectID)
}

func TestConfig_Init_ReadConfigError(t *testing.T) {
	setupLog := newMockLogger()

	// Test with an invalid config file path
	config := NewConfig()
	err := config.Init(setupLog, "invalid-path.yaml")

	// Expect an error for invalid config path
	assert.Error(t, err, "Init should return error for invalid config file path")
}

func TestConfig_Init_UnmarshalError(t *testing.T) {
	setupLog := newMockLogger()

	// Create a temporary config file with invalid YAML content
	tmpFile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }() // Clean up after test

	// Write some invalid YAML content (to trigger unmarshal error)
	invalidConfigContent := `
	invalid_yaml_content: [unterminated_string
`
	if _, err := tmpFile.Write([]byte(invalidConfigContent)); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}
	_ = tmpFile.Close()

	config := NewConfig()
	err = config.Init(setupLog, tmpFile.Name())

	// Expect an error due to invalid YAML content
	assert.Error(t, err, "Init should return error for invalid YAML content")
}

func TestConfig_NewConfig_Defaults(t *testing.T) {
	config := NewConfig()

	// Check the default values for NewConfig
	assert.NotNil(t, config)
	assert.Equal(t, "configDriver,metadataService", config.Metadata.SearchOrder)
}

// glb.console.vngcloud.vn answers every call with a 301 to glb.console.greennode.ai, and Go drops
// the Authorization header on a cross-host redirect, so a controller still pointed at the old host
// gets 401 on every GLB call. Existing installs carry no glbURL, so the default has to be the host
// that actually serves the API.
func TestGLBURLDefaultsToTheGreenNodeHost(t *testing.T) {
	assert.Equal(t, "https://glb.console.greennode.ai/glb-controller/", AuthOpts{}.GetGLBURL())
	assert.NotContains(t, AuthOpts{}.GetGLBURL(), "vngcloud.vn")
}

func TestGLBURLFromConfigWins(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "glb-url-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString("global:\n  glbURL: \"https://glb.example.test/glb-controller\"\n"); err != nil {
		t.Fatalf("Failed to write to temp config file: %v", err)
	}

	config := NewConfig()
	assert.NoError(t, config.Init(newMockLogger(), tmpFile.Name()))
	assert.Equal(t, "https://glb.example.test/glb-controller", config.Global.GetGLBURL())
}
