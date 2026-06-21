// Package render holds rendering helpers shared by the editor and viewer TUIs.
package render

import "github.com/charmbracelet/glamour"

// YAMLFence wraps yaml in a ```yaml code fence and renders it through r for
// syntax-highlighted display. It returns yaml unchanged when r is nil, yaml is
// empty, or rendering fails. Callers apply their own surrounding trimming.
func YAMLFence(yaml string, r *glamour.TermRenderer) string {
	if r == nil || yaml == "" {
		return yaml
	}
	out, err := r.Render("```yaml\n" + yaml + "\n```")
	if err != nil {
		return yaml
	}
	return out
}
