package v1alpha1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// CRDs live in two places and must stay in sync (see CLAUDE.md): the bases are what
// `kubectl apply` / Helm install, the second copy is go:embed'ed into the binary and
// applied by the controller at startup. If they diverge the API server silently strips
// fields when the controller patches Status, so the invariant below is checked against
// both.
var crdDirs = []string{
	"../../config/crd/bases",
	"../../pkg/k8s/apis/vks.vngcloud.vn/crds",
}

// TestCreatedResourceListsAreKeyedById guards an invariant of every status list that
// records cloud resources the controller created: it must be keyed by "id", never by
// "name".
//
// Cloud resource names are derived deterministically from namespace/service/port, so
// they are only unique within a single load balancer. When an Ingress is re-pointed at
// a different load balancer (the "migrate" flow), the old and the new resource share a
// name but not an id. A list keyed by "name" makes that state unrepresentable: the API
// server rejects the whole status patch with
//
//	status.createdPools[3]: Duplicate value: map[string]interface {}{"name":"vks-..."}
//
// which aborts the reconcile and leaves the object permanently stuck. Keying by "id"
// keeps both entries representable so the reconcile can proceed and tear the old ones
// down.
func TestCreatedResourceListsAreKeyedById(t *testing.T) {
	checked := 0
	for _, dir := range crdDirs {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err, "cannot read generated CRD directory %s", dir)

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}

			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)

			var crd apiextv1.CustomResourceDefinition
			require.NoError(t, yaml.Unmarshal(raw, &crd), "cannot parse %s/%s", dir, e.Name())

			for _, version := range crd.Spec.Versions {
				if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
					continue
				}
				status, ok := version.Schema.OpenAPIV3Schema.Properties["status"]
				if !ok {
					continue
				}
				checked += assertCreatedListsKeyedById(t,
					dir+" "+crd.Spec.Names.Kind+"/"+version.Name+".status", &status)
			}
		}
	}

	assert.Greater(t, checked, 0,
		"no created* status lists found - the walker or the CRD layout changed")
}

// assertCreatedListsKeyedById walks a schema and asserts every "created*" map-list is
// keyed by "id". Returns how many such lists it checked.
func assertCreatedListsKeyedById(t *testing.T, path string, schema *apiextv1.JSONSchemaProps) int {
	t.Helper()

	checked := 0
	for name, prop := range schema.Properties {
		child := path + "." + name
		isCreatedMapList := strings.HasPrefix(name, "created") &&
			prop.XListType != nil && *prop.XListType == "map"

		if isCreatedMapList {
			checked++
			assert.Equal(t, []string{"id"}, prop.XListMapKeys,
				"%s must be keyed by id: names are only unique within one load balancer, "+
					"so keying by name makes the migrate flow unrepresentable", child)
		}

		// Recurse into nested objects and into list items (e.g. createdListeners[].createdPolicies).
		nested := prop
		checked += assertCreatedListsKeyedById(t, child, &nested)
		if prop.Items != nil && prop.Items.Schema != nil {
			item := *prop.Items.Schema
			checked += assertCreatedListsKeyedById(t, child+"[]", &item)
		}
	}
	return checked
}
