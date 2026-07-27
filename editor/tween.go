package editor

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// animFrame is the interval between hint-animation frames (~60fps). Terminal
// rows are coarse, so the tween resolves to only a handful of distinct heights;
// the frame rate exists to keep the easing curve smooth in time, not in space.
const animFrame = 16 * time.Millisecond

// tween interpolates an int between two values over a duration, easing in and
// out so the motion accelerates away from the start and settles into the end.
//
// The interpolation is adapted from the window minimize/restore animation in
// github.com/Gaurav-Gosain/tuios (MIT). That version drives four axes at once
// because its windows float in a compositor and travel toward a dock; yedit's
// hint panel is stacked in the right column and can only change height, so this
// is reduced to a single axis. The curve is not the original's - see
// easeInOutQuad for why a gentler one suits cell-quantised output.
//
// The zero value is inactive: at reports the tween as finished and callers fall
// back to the panel's settled height.
type tween struct {
	from, to int
	cur      int // value sampled by the last advance; the height the frame draws
	start    time.Time
	dur      time.Duration
}

// active reports whether t is currently in flight.
func (t tween) active() bool { return t.dur > 0 }

// advance samples the curve at now and latches the result into cur, reporting
// whether the tween has finished.
//
// The renderer reads cur rather than calling at itself, so every height derived
// during one frame comes from a single sample. Sampling per read instead would
// let the clock cross a step between two reads of the same frame and lay the
// hint panel out one row apart from the space the preview left for it.
func (t *tween) advance(now time.Time) bool {
	v, done := t.at(now)
	t.cur = v
	return done
}

// at returns the interpolated value at now, and whether the tween has finished.
// A finished tween reports its end value exactly, so the panel always lands on
// the height the static layout would have chosen.
func (t tween) at(now time.Time) (int, bool) {
	if t.dur <= 0 {
		return t.to, true
	}
	p := float64(now.Sub(t.start)) / float64(t.dur)
	if p >= 1 {
		return t.to, true
	}
	if p < 0 {
		p = 0
	}
	return t.from + int(math.Round(float64(t.to-t.from)*easeInOutQuad(p))), false
}

// easeInOutQuad maps linear progress in [0,1] onto a quadratic ease-in-out
// curve: it still accelerates away from the start and settles into the end, but
// far less sharply than a cubic.
//
// The gentler curve is deliberate, and it is where this diverges from the tuios
// original. Output here is quantised to whole terminal cells, so an aggressive
// curve reads as stutter rather than as speed: a cubic's flat ends freeze the
// height for two or three consecutive frames, then its steep middle skips rows
// through the middle. Measured over the same 180ms, the quadratic never repeats
// a height more than once.
func easeInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	p := -2*t + 2
	return 1 - p*p/2
}

// hintAnimTickMsg advances an in-flight hint panel animation. block marks a
// tick belonging to the active block editor rather than the root list, so the
// root Update can route it to the right owner.
type hintAnimTickMsg struct{ block bool }

// hintAnimTick schedules the next animation frame. The loop is self-cancelling:
// the handler stops rescheduling once the tween finishes, so an idle editor
// emits no ticks at all.
func hintAnimTick(block bool) tea.Cmd {
	return tea.Tick(animFrame, func(time.Time) tea.Msg { return hintAnimTickMsg{block: block} })
}

// startTween builds the tween carrying a panel from its current on-screen
// height to target over dur. It returns the zero tween when animation is
// disabled (dur <= 0) or there is no distance to cover, which makes the caller
// fall through to an instant toggle.
func startTween(from, target int, dur time.Duration) tween {
	if dur <= 0 || from == target {
		return tween{}
	}
	return tween{from: from, to: target, cur: from, start: time.Now(), dur: dur}
}

// handleHintAnimTick advances an in-flight hint animation by one frame.
//
// Block-editor ticks are routed straight to the active editor instead of going
// through handleModeUpdate, so a frame still lands while a confirm alert is
// layered over the editor. A tick that arrives after its editor was popped off
// the stack finds an inactive tween on whatever editor is now on top and stops
// there, which is why no stack index needs to travel with the message.
func (m model) handleHintAnimTick(msg hintAnimTickMsg) (tea.Model, tea.Cmd) {
	if msg.block {
		top := m.topBE()
		if top == nil {
			return m, nil
		}
		be, cmd := top.advanceHintAnim()
		return m.withTopBE(be), cmd
	}

	if !m.hintAnim.active() {
		return m, nil
	}
	if m.hintAnim.advance(time.Now()) {
		m.hintAnim = tween{}
		// Settled: run the full relayout once so the preview renderer and its
		// content are rebuilt for the final height.
		return m.relayout(), nil
	}
	return m.relayoutHeights(), hintAnimTick(false)
}

// advanceHintAnim is the block editor's counterpart to handleHintAnimTick.
func (be blockEditState) advanceHintAnim() (blockEditState, tea.Cmd) {
	if !be.hintAnim.active() {
		return be, nil
	}
	done := be.hintAnim.advance(time.Now())
	if done {
		be.hintAnim = tween{}
	}
	// editorH shrinks and grows with the hint panel, and the textarea keeps its
	// height until told otherwise, so it has to be resized on every frame.
	be.yamlEditor.SetHeight(be.editorH() - 1)
	if done {
		return be, nil
	}
	return be, hintAnimTick(true)
}
