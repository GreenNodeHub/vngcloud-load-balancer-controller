package vngcloud_repo

import (
	"strings"
	"testing"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
)

// The SDK appends "v1" to the GLB endpoint without a separator, so the base must end with a slash
// whether or not the configured value has one.
func TestGLBEndpointEndsWithSlash(t *testing.T) {
	for _, configured := range []string{
		"",
		"https://glb.example.test/glb-controller",
		"https://glb.example.test/glb-controller/",
	} {
		cfg := config.NewConfig()
		cfg.Global.GLBURL = configured

		got := glbEndpoint(cfg)
		if !strings.HasSuffix(got, "/glb-controller/") {
			t.Errorf("glbEndpoint(%q) = %q, want a base ending in /glb-controller/", configured, got)
		}
	}
}
