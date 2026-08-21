package v1alpha1

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every created* status list is a map-list keyed by id (see crd_schema_test.go), and the
// generated CRDs mark that id required. So the key must always be serialised: with
// `json:"id,omitempty"` an entry whose id is empty goes out without the field, and the API
// server rejects the whole status patch rather than the one entry - which wedges the LBC for
// good, the same way a duplicate key did before the key was changed to id.
//
// The guard against ever recording an empty id lives in the status helpers; this asserts the
// other half, that the shape on the wire cannot hide the key.
func TestCreatedResourceIdIsAlwaysSerialised(t *testing.T) {
	cases := map[string]any{
		"CreatedPool":        CreatedPool{},
		"CreatedListener":    CreatedListener{},
		"CreatedPolicy":      CreatedPolicy{},
		"CreatedCertificate": CreatedCertificate{},
		"CreatedGlobalPool":  CreatedGlobalPool{},
	}

	for name, zero := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(zero)
			require.NoError(t, err)

			var out map[string]any
			require.NoError(t, json.Unmarshal(raw, &out))
			assert.Contains(t, out, "id",
				`%s must serialise its id even when empty: it is the map-list key, and the CRD `+
					`marks it required, so omitting it makes the API server reject the whole patch`, name)
		})
	}
}

// The tag and the marker have to agree. `+required` puts id in the CRD's required list, so a
// Go tag that omits it when empty guarantees a rejected patch rather than a validation error
// anyone can read.
func TestCreatedResourceIdTagHasNoOmitempty(t *testing.T) {
	for _, file := range []string{"loadbalancerconfig_types.go", "globalloadbalancerconfig_types.go"} {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "Id string `json:\"id,omitempty\"`",
			"%s: a map-list key must not be omitempty", file)
		assert.True(t, strings.Contains(string(raw), "Id string `json:\"id\"`"),
			"%s: expected at least one map-list key declared as json:\"id\"", file)
	}
}
