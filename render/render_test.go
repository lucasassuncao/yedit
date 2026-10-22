package render

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYAMLFenceWithoutRendererIsPlain(t *testing.T) {
	assert.Equal(t, "a: 1", YAMLFence("a: 1", nil))
}

func TestYAMLFenceHighlights(t *testing.T) {
	r := NewPreviewRenderer(80)
	require.NotNil(t, r)

	assert.Empty(t, YAMLFence("", r), "nothing to render")

	out := YAMLFence("a: 1", r)
	assert.NotEqual(t, "a: 1", out, "the fence is rendered, not passed through")
	assert.Contains(t, ansi.Strip(out), "a: 1")
}
