package editor

import "github.com/lucasassuncao/yedit/theme"

const (
	headerLines    = 1
	statusBarLines = 2
)

func (m model) relayout() model {
	var previewW int
	m.listW, previewW = theme.TwoColumnWidths(m.width)
	m.innerH = m.height - headerLines - statusBarLines - 2
	if m.innerH < 1 {
		m.innerH = 1
	}
	m.list = m.list.SetHeight(m.innerH)
	m.preview.Width = previewW - 2
	ph := m.innerH
	if m.showHint {
		ph = m.previewPanelH()
	}
	if ph < 1 {
		ph = 1
	}
	m.preview.Height = ph
	m.previewRenderer = newPreviewRenderer(m.preview.Width)
	m = m.refreshPreview()
	return m
}

// hintPanelH is the content height of the Hint/Example panel when it shares the
// right column with the preview. Mirrors blockEditState.hintH: ~1/3, floored.
func (m model) hintPanelH() int {
	total := m.innerH - 2 // extra border row from stacking two panels
	h := total / 3
	if h < 5 {
		h = 5
	}
	if total-h < 5 {
		h = total - 5
	}
	if h < 0 {
		h = 0
	}
	return h
}

// previewPanelH is the content height of the preview when the hint panel shares
// the right column.
func (m model) previewPanelH() int {
	h := m.innerH - 2 - m.hintPanelH()
	if h < 0 {
		h = 0
	}
	return h
}
