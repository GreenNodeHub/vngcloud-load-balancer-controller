package crdhelpers

import (
	"testing"

	"github.com/blang/semver/v4"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// needsUpdateV1 decides whether the controller upgrades the CRDs it embeds, and it compares two
// versions that are parsed by different functions: the label already on the cluster goes through
// versioncheck.Version, while the chart version arrives as semver.MustParse from register.go.
//
// versioncheck.Version keeps a prerelease only when it contains rc, beta, alpha or snapshot -
// anything else is stripped down to major.minor.patch. So a chart of 0.4.0-dev.<sha> is written
// to the label as-is, read back as plain 0.4.0, and 0.4.0 outranks 0.4.0-dev.<anything>: the next
// dev build is skipped, silently, and every status field added since is stripped by the API
// server with nothing in the log to say why.
//
// This is not hypothetical. It is how a cluster sat on the 0.3.23 schema while running a build of
// main, and how a second dev build then failed to upgrade a cluster the first one had.
//
// The test exists to pin the rule the dev pipeline has to follow: a dev version must differ in
// major/minor/patch, not in a prerelease.

const schemaVersionLabel = "vks.vngcloud.vn/crd-schema-version"

func clusterCRDAt(version string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{schemaVersionLabel: version}},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1alpha1", Schema: &apiextensionsv1.CustomResourceValidation{}},
			},
		},
	}
}

func TestNeedsUpdateAcrossTheVersionsThePipelinesProduce(t *testing.T) {
	tests := []struct {
		name       string
		onCluster  string
		installing string
		want       bool
	}{
		{
			name:       "release over an older release",
			onCluster:  "0.3.23",
			installing: "0.3.25",
			want:       true,
		},
		{
			name:       "first dev build over a release",
			onCluster:  "0.3.23",
			installing: "0.4.0-dev.7801178",
			want:       true,
		},
		{
			// The one that bites: both sides are the same major.minor.patch once the cluster's
			// prerelease has been stripped, so the cluster reads as newer than the chart.
			name:       "second dev build over the first - a prerelease cannot upgrade a prerelease",
			onCluster:  "0.4.0-dev.7801178",
			installing: "0.4.0-dev.1c0b857",
			want:       false,
		},
		{
			// Which is why the dev pipeline numbers its builds in the patch instead.
			name:       "dev builds numbered in the patch component",
			onCluster:  "0.4.7",
			installing: "0.4.8",
			want:       true,
		},
		{
			name:       "a real release still wins over a dev build below it",
			onCluster:  "0.4.7",
			installing: "0.5.0",
			want:       true,
		},
		{
			name:       "installing the same version changes nothing",
			onCluster:  "0.4.8",
			installing: "0.4.8",
			want:       false,
		},
		{
			name:       "an unparseable label is treated as out of date",
			onCluster:  "not-a-version",
			installing: "0.4.8",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsUpdateV1(clusterCRDAt(tt.onCluster), schemaVersionLabel, semver.MustParse(tt.installing))
			if got != tt.want {
				t.Errorf("cluster at %q, installing %q: needsUpdateV1 = %v, want %v",
					tt.onCluster, tt.installing, got, tt.want)
			}
		})
	}
}

// A CRD that has never been labelled is always brought up to date, whatever it holds.
func TestNeedsUpdateWhenTheClusterCRDHasNoSchemaVersion(t *testing.T) {
	crd := clusterCRDAt("0.4.8")
	crd.Labels = nil

	if !needsUpdateV1(crd, schemaVersionLabel, semver.MustParse("0.4.8")) {
		t.Error("a CRD with no schema-version label must be updated, or a hand-applied CRD is never corrected")
	}
}
