package editor

import "github.com/lucasassuncao/yedit/theme"

const (
	headerLines   = 1
	feedbackLines = 1
)

func (m model) relayout() model {
	var previewW int
	m.listW, previewW = theme.TwoColumnWidths(m.width)

	previewFocused := m.mode == panePreview
	_, legendLines := renderLegend(m.help, listKeyMapFor(m, previewFocused), m.width-1)
	if legendLines < 1 {
		legendLines = 1
	}

	m.innerH = m.height - headerLines - feedbackLines - legendLines - 2
	if m.innerH < 1 {
		m.innerH = 1
	}
	m.list = m.list.SetHeight(m.innerH)
	m.preview.SetWidth(previewW - 2)
	ph := m.innerH
	if m.showHint {
		ph = m.previewPanelH()
	}
	if ph < 1 {
		ph = 1
	}
	m.preview.SetHeight(ph)
	wrap := m.preview.Width() - previewGutterWidth
	if wrap < 1 {
		wrap = 1
	}
	m.previewRenderer = newPreviewRenderer(wrap)
	m = m.refreshPreview()
	// After a resize the viewport height may have shrunk; clamp the scroll so
	// it cannot exceed the new view boundary.
	if m.preview.YOffset() > m.preview.TotalLineCount()-m.preview.Height() {
		maxOffset := m.preview.TotalLineCount() - m.preview.Height()
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.preview.SetYOffset(maxOffset)
	}
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
