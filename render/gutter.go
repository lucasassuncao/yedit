package render

import (
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/bezel/component/viewport"
	"github.com/lucasassuncao/bezel/draw"
)

// ViewportGutter is a viewport line-number column in the shape of bezel's
// draw.Gutter, so the preview lines up with the YAML editor. The viewport
// probes the width with a zero row, hence the fixed format.
func ViewportGutter(style lipgloss.Style) func(viewport.GutterRow) string {
	return func(row viewport.GutterRow) string {
		if row.Line >= row.Lines || row.Continued {
			return style.Render("     │ ")
		}
		return draw.Gutter(row.Line+1, style)
	}
}
