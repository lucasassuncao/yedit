package render

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/component/viewport"
	"github.com/lucasassuncao/bezel/draw"
)

// A viewport's gutter lines up with draw.Gutter, and past the last line keeps
// the same width with no number, so the rule runs straight down.
func TestViewportGutterMatchesGutterAndBlanksPastTheEnd(t *testing.T) {
	st := lipgloss.NewStyle()
	g := ViewportGutter(st)
	require.Equal(t, draw.Gutter(3, st), g(viewport.GutterRow{Line: 2, Lines: 5}))
	past := ansi.Strip(g(viewport.GutterRow{Line: 5, Lines: 5}))
	require.Equal(t, "     │ ", past)
	require.Equal(t, draw.GutterWidth, ansi.StringWidth(past))
}
