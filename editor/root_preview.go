package editor

import (
	"os"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"

	"github.com/lucasassuncao/yedit/render"
)

func (m model) togglePreviewPane() (tea.Model, tea.Cmd) {
	if m.mode == panePreview {
		m = m.enterList()
		m.statusMsg = ""
		return m, nil
	}
	m = m.enterPreview()
	return m.withStatus("Viewing YAML - ↑/↓ scroll, Tab/Esc back to list.")
}

func (m model) syncView() model {
	m = m.refreshPreview()
	m.list = m.list.Rebuild(m.doc.Blocks())
	m = m.scrollPreviewToSelected()
	return m
}

// scrollPreviewToSelected scrolls the read-only preview so the YAML for the
// selected top-level block sits near the top, letting list navigation track the
// document. Applies only in the list pane and only for keys present in the file.
// AVAILABLE items (not yet in the file) are silently skipped — do not reset the
// scroll to 0 when the selected item is absent from the document.
// The scroll is line-based, so it can drift slightly when long lines above the
// block wrap.
func (m model) scrollPreviewToSelected() model {
	if m.mode != paneList {
		return m
	}
	it := m.list.SelectedItem()
	if it == nil || !it.Existing {
		// AVAILABLE or separator item — leave scroll unchanged.
		return m
	}
	for _, b := range m.doc.Blocks() {
		if b.Key == it.Key {
			m.preview.SetYOffset(b.Line - 1)
			return m
		}
	}
	// Key not found in doc.Blocks() (e.g. passthrough or stale list) — leave
	// scroll unchanged rather than snapping to offset 0.
	return m
}

// newPreviewRenderer builds a glamour renderer that word-wraps to wrap columns.
// It starts from the dark style (or the colorless ASCII style under NO_COLOR)
// and trims glamour's default chrome: the document and code-block left margins
// stack to ~4 columns and the block prefix/suffix add blank lines, all wasteful
// inside a panel that already has its own border. A single-column margin is
// kept. Returns nil on error, in which case renderPreviewYAML falls back to
// plain text.
func newPreviewRenderer(wrap int) *glamour.TermRenderer {
	cfg := styles.DarkStyleConfig
	if os.Getenv("NO_COLOR") != "" {
		cfg = styles.NoTTYStyleConfig
	}
	one, zero := uint(1), uint(0)
	cfg.Document.Margin = &one
	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	cfg.CodeBlock.Margin = &zero

	r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(wrap))
	if err != nil {
		return nil
	}
	return r
}

// renderPreviewYAML renders raw YAML through r (wrapped in a markdown code fence)
// for syntax-highlighted display. Falls back to the plain text when r is nil or
// rendering fails.
func renderPreviewYAML(raw string, r *glamour.TermRenderer) string {
	raw = strings.TrimSuffix(raw, "\n")
	if r == nil || raw == "" {
		return raw
	}
	out := render.YAMLFence(raw, r)
	if out == raw {
		return raw // rendering failed - YAMLFence returned the input unchanged
	}
	return trimBlankLines(out)
}

var ansiEscapeRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// trimBlankLines drops leading and trailing whitespace-only lines - glamour
// emits a padded blank line around the code block - while leaving any interior
// blank lines intact. It is ANSI-aware so colored padding still reads as blank.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(l string) bool {
		return strings.TrimSpace(ansiEscapeRE.ReplaceAllString(l, "")) == ""
	}
	start, end := 0, len(lines)
	for start < end && blank(lines[start]) {
		start++
	}
	for end > start && blank(lines[end-1]) {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// clampLines truncates s to at most maxLines newline-separated lines so that
// content passed to RenderTitledPanel never overflows its allocated height.
// lipgloss.Height() is a minimum, not a cap - without this, a tall hint or
// preview would push the right column taller than the left.
func clampLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}

// refreshPreview re-renders the document into the read-only preview viewport,
// syntax-highlighted and wrapped to the current panel width.
func (m model) refreshPreview() model {
	m.preview.SetContent(renderPreviewYAML(string(m.doc.Raw()), m.previewRenderer))
	return m
}
