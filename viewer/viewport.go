package viewer

import "github.com/charmbracelet/glamour"

// renderYAML wraps yaml in a markdown code fence and runs it through the
// provided glamour renderer for syntax highlighting. Falls back to the raw
// YAML if the renderer is nil or rendering fails.
func renderYAML(yaml string, r *glamour.TermRenderer) string {
	if r == nil || yaml == "" {
		return yaml
	}
	md := "```yaml\n" + yaml + "\n```\n"
	out, err := r.Render(md)
	if err != nil {
		return yaml
	}
	return out
}
