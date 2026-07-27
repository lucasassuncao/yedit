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
	m.preview.SetHeight(m.previewViewportH())
	wrap := m.preview.Width() - previewGutterWidth
	if wrap < 1 {
		wrap = 1
	}
	m.previewRenderer = newPreviewRenderer(wrap)
	m = m.refreshPreview()
	return m.clampPreviewScroll()
}

// relayoutHeights re-derives only the height-dependent part of the layout. It
// is the per-frame path for the hint animation: unlike relayout it leaves
// previewRenderer and the rendered preview content alone, which would otherwise
// re-run the whole document through glamour on every frame. That is safe here
// because showing or hiding the hint panel changes heights only - the column
// widths, and so the wrap width the renderer was built for, do not move.
func (m model) relayoutHeights() model {
	m.preview.SetHeight(m.previewViewportH())
	return m.clampPreviewScroll()
}

// previewViewportH is the height given to the preview viewport: the full inner
// height when the preview owns the right column alone, otherwise what is left
// after the hint panel takes its share.
func (m model) previewViewportH() int {
	ph := m.innerH
	if m.hintVisible() {
		ph = m.previewPanelH()
	}
	if ph < 1 {
		ph = 1
	}
	return ph
}

// clampPreviewScroll pulls the preview's scroll offset back inside the viewport
// after its height shrank (a resize, or the hint panel easing open).
func (m model) clampPreviewScroll() model {
	if m.preview.YOffset() > m.preview.TotalLineCount()-m.preview.Height() {
		maxOffset := m.preview.TotalLineCount() - m.preview.Height()
		if maxOffset < 0 {
			maxOffset = 0
		}
		m.preview.SetYOffset(maxOffset)
	}
	return m
}

// hintVisible reports whether the Hint/Example panel is drawn at all. It stays
// true mid-animation, while the panel is on its way in or out, but goes false
// the moment the eased height reaches zero.
//
// That last part matters: theme.RenderTitledPanelWith floors every panel at 3
// rows, so a hint panel asked for height 0 (rendered as 0+2 = 2) comes back 3
// rows tall anyway. Drawing one would make the right column innerH+3 against
// the left column's innerH+2 and push the legend bar down by a row for exactly
// the frames the tween sits at zero, which reads as the legend flickering at
// the end of the transition.
func (m model) hintVisible() bool { return m.hintPanelH() > 0 }

// hintTargetH is the height the Hint/Example panel settles at once shown: ~1/3
// of the right column, floored at 5 lines and never squeezing the preview below
// 5. Mirrors blockEditState.hintTargetH.
func (m model) hintTargetH() int {
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

// hintPanelH is the height the Hint/Example panel is drawn at right now: the
// interpolated value while a show/hide tween is in flight, and the settled
// target otherwise.
//
// The floors in hintTargetH are deliberately not reapplied to the animated
// value. They describe the resting size; clamping every frame to them would
// make the panel jump straight to 5 lines instead of growing out of nothing.
func (m model) hintPanelH() int {
	if m.hintAnim.active() {
		return m.hintAnim.cur
	}
	if !m.showHint {
		return 0
	}
	return m.hintTargetH()
}

// startHintAnim eases the hint panel from the height it is currently drawn at
// towards the height implied by the new m.showHint, and reports whether a tick
// loop needs to be started. from must be sampled before m.showHint is flipped.
// With Config.AnimationDuration unset the tween stays inactive and the panel
// snaps, so an embedding app that did not ask for animation gets no timers.
func (m model) startHintAnim(from int) (model, bool) {
	running := m.hintAnim.active()
	target := 0
	if m.showHint {
		target = m.hintTargetH()
	}
	m.hintAnim = startTween(from, target, m.cfg.AnimationDuration)
	// Only spawn a loop when one is not already in flight; a rapid double
	// toggle retargets the existing tween instead of stacking a second ticker.
	return m, m.hintAnim.active() && !running
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
