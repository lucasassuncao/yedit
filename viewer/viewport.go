package viewer

import (
	"github.com/charmbracelet/glamour"

	"github.com/lucasassuncao/yedit/internal/render"
)

// renderYAML wraps yaml in a markdown code fence and runs it through the
// provided glamour renderer for syntax highlighting. Falls back to the raw
// YAML if the renderer is nil or rendering fails.
func renderYAML(yaml string, r *glamour.TermRenderer) string {
	return render.YAMLFence(yaml, r)
}
