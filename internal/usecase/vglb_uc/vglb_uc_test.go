package vglb_uc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripZoneSuffix(t *testing.T) {
	tests := []struct {
		name     string
		zone     string
		expected string
	}{
		{name: "hcm03b", zone: "hcm03b", expected: "hcm"},
		{name: "sgn01a", zone: "sgn01a", expected: "sgn"},
		{name: "han01", zone: "han01", expected: "han"},
		{name: "hcm_no_suffix", zone: "hcm", expected: "hcm"},
		{name: "empty", zone: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripZoneSuffix(tt.zone))
		})
	}
}
