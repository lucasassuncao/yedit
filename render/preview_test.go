package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewYAMLWithoutRendererDropsTheFinalNewline(t *testing.T) {
	assert.Equal(t, "a: 1\nb: 2", PreviewYAML("a: 1\nb: 2\n", nil))
	assert.Empty(t, PreviewYAML("\n", NewPreviewRenderer(80)))
}

func TestPreviewYAMLHasNoPaddingLines(t *testing.T) {
	r := NewPreviewRenderer(80)
	require.NotNil(t, r)

	lines := strings.Split(ansi.Strip(PreviewYAML("a: 1\nb: 2\n", r)), "\n")
	require.NotEmpty(t, lines)
	assert.NotEmpty(t, strings.TrimSpace(lines[0]), "no blank line above the code")
	assert.NotEmpty(t, strings.TrimSpace(lines[len(lines)-1]), "no blank line below the code")
	assert.Contains(t, lines[0], "a: 1")
}

func TestTrimBlankLinesKeepsInteriorBlanks(t *testing.T) {
	in := "\n  \n\x1b[0m  \x1b[0m\nx\n\ny\n \n"
	assert.Equal(t, "x\n\ny", trimBlankLines(in), "colored padding still reads as blank")
	assert.Empty(t, trimBlankLines(" \n\n"))
}
