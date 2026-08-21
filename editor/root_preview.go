package editor

import (
	tea "charm.land/bubbletea/v2"

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

// refreshPreview re-renders the document into the read-only preview viewport,
// syntax-highlighted and wrapped to the current panel width.
func (m model) refreshPreview() model {
	m.preview.SetContent(render.PreviewYAML(string(m.doc.Raw()), m.previewRenderer))
	return m
}
