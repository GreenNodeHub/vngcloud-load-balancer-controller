package vksvngcloudvn

import (
	"os"
	"path/filepath"
	"testing"
)

// LBC-INV-07: the CRDs the binary applies at start-up must be the ones `make manifests` generated.
func TestEmbeddedCRDsMatchGeneratedBases(t *testing.T) {
	bases, _ := filepath.Glob("../../../../config/crd/bases/vks.vngcloud.vn_*.yaml")
	if len(bases) == 0 {
		t.Fatal("no generated CRDs found")
	}
	for _, b := range bases {
		want, _ := os.ReadFile(b)
		got, err := os.ReadFile(filepath.Join("crds", filepath.Base(b)))
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: embedded copy differs from config/crd/bases - run make manifests", filepath.Base(b))
		}
	}
}
