package editor

import (
	"github.com/lucasassuncao/bezel/animation"
	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/yedit/render"
)

// listColumn is the left column of every yedit screen: a third of the
// terminal, clamped so names stay readable and the document keeps the room.
func listColumn(name string) layout.Leaf {
	return layout.Fixed(name, layout.Ratio(1, 3), layout.Min(30), layout.Max(60))
}

// rootLayout is the list screen. hintH is the hint panel's content height;
// zero means the preview owns the right column alone.
func rootLayout(hintH int) layout.Node {
	right := layout.Node(layout.Fill("preview"))
	if hintH > 0 {
		right = layout.Rows(layout.Fill("preview"), layout.Fixed("hint", layout.Lines(hintH+2)).Info())
	}
	return layout.Columns(listColumn("list"), right)
}

func docPresetLayout() layout.Node {
	return layout.Columns(listColumn("presets"), layout.Fill("preview"))
}

func (m model) currentLayout() layout.Node {
	if m.mode == paneDocPreset {
		return docPresetLayout()
	}
	return rootLayout(m.hintPanelH())
}

// relayout re-resolves the shell for the current mode and hint height, then
// sizes the list and the preview viewport from what it placed.
func (m model) relayout() model {
	m = m.withActions() // first: the legend's rows decide what currentLayout measures
	m.sh = m.sh.SetLayout(m.currentLayout())

	m.list = m.list.SetHeight(m.innerH())
	preview := draw.InnerRect(m.sh.Rect("preview"))
	m.preview.SetWidth(preview.W)
	m.preview.SetHeight(preview.H)
	wrap := max(1, preview.W-draw.GutterWidth)
	m.previewRenderer = render.NewPreviewRenderer(wrap)
	m = m.refreshPreview()
	return m.clampPreviewScroll()
}

// relayoutHeights is the per-frame path for the hint animation: it moves the
// split without rebuilding the glamour renderer, which the width alone decides.
func (m model) relayoutHeights() model {
	m.sh = m.sh.SetLayout(m.currentLayout())
	m.preview.SetHeight(draw.InnerRect(m.sh.Rect("preview")).H)
	return m.clampPreviewScroll()
}

// innerH is the content height of the list panel: what every other pane on
// the screen shares. Measured on the list layout, since the shell may still
// hold another screen's (the preset picker has no "list").
func (m model) innerH() int {
	return draw.InnerRect(m.sh.SetLayout(rootLayout(0)).Rect("list")).H
}

// clampPreviewScroll pulls the preview's scroll offset back inside the viewport
// after its height shrank (a resize, or the hint panel easing open).
func (m model) clampPreviewScroll() model {
	m.preview.SetYOffset(m.preview.YOffset()) // SetYOffset clamps to the content
	return m
}

// hintVisible reports whether the Hint/Example panel is drawn at all. It stays
// true mid-animation but goes false the moment the eased height reaches zero,
// so the layout drops the leaf instead of drawing a title-only panel.
func (m model) hintVisible() bool { return m.hintPanelH() > 0 }

// hintTargetH is the height the Hint/Example panel settles at once shown: ~1/3
// of the right column, floored at 5 lines and never squeezing the preview below
// 5. Mirrors blockEditState.hintTargetH.
func (m model) hintTargetH() int {
	total := m.innerH() - 2 // extra border row from stacking two panels
	h := max(total/3, 5)
	if total-h < 5 {
		h = total - 5
	}
	return max(h, 0)
}

// hintPanelH is the height the Hint/Example panel is drawn at right now: the
// interpolated value while a show/hide tween is in flight, the settled target
// otherwise. The floors are the resting size only, so the panel grows from 0.
func (m model) hintPanelH() int {
	if m.hintAnim.Active() {
		return m.hintAnim.Cur
	}
	if !m.showHint {
		return 0
	}
	return m.hintTargetH()
}

// startHintAnim eases the hint panel from the height it is currently drawn at
// towards the height implied by the new m.showHint, and reports whether a tick
// loop needs to be started. from must be sampled before m.showHint is flipped.
func (m model) startHintAnim(from int) (model, bool) {
	running := m.hintAnim.Active()
	target := 0
	if m.showHint {
		target = m.hintTargetH()
	}
	m.hintAnim = animation.New(from, target, m.cfg.AnimationDuration)
	// A rapid double toggle retargets the existing tween instead of stacking a
	// second ticker.
	return m, m.hintAnim.Active() && !running
}
