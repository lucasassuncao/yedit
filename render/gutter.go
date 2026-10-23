package render

import (
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/bezel/draw"
)

// ViewportGutter is a viewport line-number column in the shape of bezel's
// draw.Gutter, so the preview lines up with the YAML editor. The viewport
// probes the width with a zero context, hence the fixed format.
func ViewportGutter(style lipgloss.Style) viewport.GutterFunc {
	return func(ctx viewport.GutterContext) string {
		if ctx.Index >= ctx.TotalLines {
			return style.Render("     │ ")
		}
		return draw.Gutter(ctx.Index+1, style)
	}
}
