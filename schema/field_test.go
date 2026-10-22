package schema_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/lucasassuncao/yedit/schema"
)

func TestPresentationYAMLRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		want schema.Presentation
	}{
		{"default", schema.PresentationDefault},
		{"flat", schema.PresentationFlat},
		{"inline", schema.PresentationInline},
		{"overlay", schema.PresentationOverlay},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p schema.Presentation
			require.NoError(t, yaml.Unmarshal([]byte(tc.name), &p))
			assert.Equal(t, tc.want, p)

			out, err := yaml.Marshal(p)
			require.NoError(t, err)
			assert.Equal(t, tc.name+"\n", string(out))
		})
	}
}

func TestPresentationEmptyNameIsDefault(t *testing.T) {
	var holder struct {
		P schema.Presentation `yaml:"p"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(`p: ""`), &holder))
	assert.Equal(t, schema.PresentationDefault, holder.P)
}

func TestPresentationUnknownNameIsAnError(t *testing.T) {
	var p schema.Presentation
	assert.ErrorContains(t, yaml.Unmarshal([]byte("sideways"), &p), `unknown presentation "sideways"`)
}
