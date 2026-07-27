package editor

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
)

// hintProbeConfig is a minimal schema with one struct block, so a test can
// exercise the hint panel on both the root list and a block editor.
type hintProbeConfig struct {
	Server serverProbe `yaml:"server"`
}

// newHintModel builds a sized root model with hints enabled and the given
// animation duration (0 disables animation).
func newHintModel(t *testing.T, dur time.Duration) model {
	t.Helper()
	m, err := newModel(Config{
		Path:              filepath.Join(t.TempDir(), "probe.yaml"),
		Schema:            &hintProbeConfig{},
		EnableHints:       true,
		Metadata:          MetadataFunc(func(string, string) FieldMeta { return FieldMeta{} }),
		AnimationDuration: dur,
	})
	require.NoError(t, err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return updated.(model)
}

// pressHint sends the hint-toggle key through the real Update path, which is
// where the animation tick loop is started.
func pressHint(m model) (model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	return updated.(model), cmd
}

func TestTweenAtEndpointsAndMidpoint(t *testing.T) {
	is := assert.New(t)

	start := time.Now()
	tw := tween{from: 0, to: 12, start: start, dur: 100 * time.Millisecond}

	v, done := tw.at(start)
	is.Equal(0, v, "at t=0 the tween sits on its start value")
	is.False(done, "a tween at t=0 is not finished")

	// The curve is symmetric about 0.5, so the midpoint is the midpoint.
	v, done = tw.at(start.Add(50 * time.Millisecond))
	is.Equal(6, v, "at t=0.5 the eased value is half the distance")
	is.False(done, "a tween at t=0.5 is not finished")

	v, done = tw.at(start.Add(100 * time.Millisecond))
	is.Equal(12, v, "a finished tween lands exactly on its end value")
	is.True(done, "at t=1 the tween is finished")

	v, done = tw.at(start.Add(time.Second))
	is.Equal(12, v, "overshooting time does not overshoot the value")
	is.True(done, "past t=1 the tween is finished")
}

// TestTweenEasesRatherThanInterpolatesLinearly is what separates this from a
// plain lerp: the eased curve starts slow, so the first quarter of the time
// covers noticeably less than a quarter of the distance.
func TestTweenEasesRatherThanInterpolatesLinearly(t *testing.T) {
	is := assert.New(t)
	start := time.Now()
	tw := tween{from: 0, to: 100, start: start, dur: 100 * time.Millisecond}

	v, _ := tw.at(start.Add(25 * time.Millisecond))
	is.Less(v, 25, "ease-in must lag a linear interpolation early on")

	v, _ = tw.at(start.Add(75 * time.Millisecond))
	is.Greater(v, 75, "ease-out must lead a linear interpolation late on")
}

func TestZeroTweenIsInactive(t *testing.T) {
	is := assert.New(t)
	var tw tween
	is.False(tw.active(), "the zero tween is inactive")
	_, done := tw.at(time.Now())
	is.True(done, "the zero tween reports as finished")
}

func TestStartTweenDeclinesWhenPointless(t *testing.T) {
	is := assert.New(t)
	is.False(startTween(0, 10, 0).active(), "duration 0 means no animation")
	is.False(startTween(7, 7, time.Second).active(), "no distance means no animation")
	is.True(startTween(0, 10, time.Second).active(), "a real move animates")
}

// TestHintPanelHMatchesTargetWhenIdle pins the invariant that keeps every
// pre-existing layout test valid: with no animation in flight the drawn height
// is exactly the height the static layout always computed.
func TestHintPanelHMatchesTargetWhenIdle(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, 0)

	is.True(m.showHint, "hints start shown when EnableHints is set")
	is.False(m.hintAnim.active(), "no animation is in flight at rest")
	is.Equal(m.hintTargetH(), m.hintPanelH(), "idle height is the settled target")
	is.True(m.hintVisible(), "a shown panel is visible")
}

// TestToggleHintsWithoutAnimationIsInstant guards the library default: an
// embedding app that never sets AnimationDuration must get no timer messages.
func TestToggleHintsWithoutAnimationIsInstant(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, 0)

	m, cmd := pressHint(m)

	is.False(m.showHint, "the toggle flipped the flag")
	is.False(m.hintAnim.active(), "no tween is started when animation is off")
	is.Nil(cmd, "no tick loop is spawned when animation is off")
	is.Equal(0, m.hintPanelH(), "the hidden panel takes no height immediately")
	is.False(m.hintVisible(), "the hidden panel is not drawn")
}

// TestToggleHintsAnimatesAndSettles walks a full hide transition: the panel
// keeps occupying (shrinking) space while in flight, and the tick loop stops
// on its own once the tween lands.
func TestToggleHintsAnimatesAndSettles(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	const dur = 60 * time.Millisecond
	m := newHintModel(t, dur)
	shownH := m.hintPanelH()
	must.Positive(shownH, "the panel starts with a real height")

	m, cmd := pressHint(m)
	must.NotNil(cmd, "hiding with animation on spawns a tick loop")
	must.True(m.hintAnim.active(), "a tween is in flight")

	// Mid-flight: the flag is already off, but the panel is still on screen and
	// smaller than it was. This is the case a plain `if m.showHint` would miss.
	is.False(m.showHint, "the flag flips immediately")
	is.True(m.hintVisible(), "the panel keeps its space while easing out")
	is.LessOrEqual(m.hintPanelH(), shownH, "the panel is shrinking, not growing")

	// The height is latched per frame, not resampled per read: the preview and
	// the hint panel must agree on how the column is split within one render,
	// however long the render takes.
	h := m.hintPanelH()
	time.Sleep(2 * animFrame)
	is.Equal(h, m.hintPanelH(), "the drawn height is stable between frames")
	is.Equal(m.innerH-2-h, m.previewPanelH(), "the two panels split the column exactly")

	// Drive frames until the tween lands, exactly as the runtime would.
	deadline := time.Now().Add(2 * time.Second)
	for m.hintAnim.active() {
		must.False(time.Now().After(deadline), "animation never settled")
		var updated tea.Model
		updated, cmd = m.handleHintAnimTick(hintAnimTickMsg{})
		m = updated.(model)
	}

	is.Nil(cmd, "the final frame stops rescheduling itself")
	is.Equal(0, m.hintPanelH(), "the panel settles fully closed")
	is.False(m.hintVisible(), "the settled panel is gone")
	is.Equal(m.innerH, m.preview.Height(), "the preview reclaims the whole column")
}

// TestHintAnimKeepsLegendBarAnchored is the regression test for the legend bar
// flickering at the end of a transition.
//
// theme.RenderTitledPanelWith floors a panel at 3 rows, so a hint panel asked
// for height 0 (0+2 = 2) still came back 3 rows tall. That made the right
// column innerH+3 against the left column's innerH+2, and the resulting overflow
// row pushed the legend past the bottom of the screen, where viewContent's final
// clampLines chopped it off. The total row count stays m.height either way -
// what moves is the legend - so this asserts the bottom row itself, on every
// frame of both directions.
func TestHintAnimKeepsLegendBarAnchored(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	m := newHintModel(t, 60*time.Millisecond)
	must.Equal(40, viewLines(m.viewContent()), "the resting layout fills the screen exactly")

	for _, phase := range []string{"hide", "show"} {
		updated, _ := m.dispatch(ToggleHints{})
		m = updated.(model)
		must.True(m.hintAnim.active(), phase+": a tween is in flight")

		// The legend text is settled for the whole phase: showHint flipped once,
		// up front. So every frame must end on the same bottom row.
		want := lastLine(m.viewContent())
		must.NotEmpty(want, phase+": the legend bar is on the bottom row")

		deadline := time.Now().Add(2 * time.Second)
		for m.hintAnim.active() {
			must.False(time.Now().After(deadline), phase+": animation never settled")
			updated, _ = m.handleHintAnimTick(hintAnimTickMsg{})
			m = updated.(model)
			is.Equal(want, lastLine(m.viewContent()),
				"%s: frame at hint height %d shifted the legend bar", phase, m.hintPanelH())
			is.Equal(40, viewLines(m.viewContent()), phase+": the view still fills the screen")
		}
	}
}

// TestZeroHeightHintPanelIsNotDrawn pins the specific condition behind the
// flicker, so a future change to hintVisible cannot quietly reintroduce it.
func TestZeroHeightHintPanelIsNotDrawn(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, time.Second)

	// Park a tween at exactly zero while still marked in flight - the state the
	// tail frames of a hide transition sit in.
	m.showHint = false
	m.hintAnim = tween{from: 8, to: 0, cur: 0, start: time.Now(), dur: time.Second}

	is.True(m.hintAnim.active(), "the tween is still in flight")
	is.Equal(0, m.hintPanelH(), "the eased height has reached zero")
	is.False(m.hintVisible(), "a zero-height hint panel must not be drawn")
	is.Equal(m.innerH, m.previewViewportH(), "the preview takes the whole column instead")
}

// viewLines counts the rendered rows of a view.
func viewLines(s string) int { return strings.Count(s, "\n") + 1 }

// lastLine returns the bottom rendered row of a view, where the legend sits.
func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

// TestHintAnimTickIsInertWhenIdle covers the stale tick: a frame that arrives
// after its tween finished (or after its block editor was popped) must not
// restart the loop.
func TestHintAnimTickIsInertWhenIdle(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, 0)

	updated, cmd := m.handleHintAnimTick(hintAnimTickMsg{})
	is.Nil(cmd, "an idle root tick does not reschedule")
	is.Equal(m.hintPanelH(), updated.(model).hintPanelH(), "an idle tick changes nothing")

	updated, cmd = m.handleHintAnimTick(hintAnimTickMsg{block: true})
	is.Nil(cmd, "a block tick with no editor open does not reschedule")
	is.Equal(paneList, updated.(model).mode, "a stray block tick does not change screens")
}

// TestHintAnimFramesDoNotRebuildPreviewRenderer is the performance contract:
// relayoutHeights must not re-run the document through glamour on every frame.
// The renderer is rebuilt only by a full relayout, so an unchanged pointer
// across the in-flight frames proves the cheap path was taken.
func TestHintAnimFramesDoNotRebuildPreviewRenderer(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	m := newHintModel(t, 60*time.Millisecond)
	updated, _ := m.dispatch(ToggleHints{})
	m = updated.(model)
	must.True(m.hintAnim.active(), "a tween is in flight")

	renderer := m.previewRenderer
	must.NotNil(renderer, "the preview renderer is built up front")

	frames := 0
	deadline := time.Now().Add(2 * time.Second)
	for m.hintAnim.active() {
		must.False(time.Now().After(deadline), "animation never settled")
		updated, _ = m.handleHintAnimTick(hintAnimTickMsg{})
		m = updated.(model)
		if m.hintAnim.active() {
			is.Same(renderer, m.previewRenderer, "in-flight frames must reuse the renderer")
		}
		frames++
	}
	is.Greater(frames, 1, "the transition spans several frames")
}

// TestBlockEditorHintAnimates mirrors the root coverage for the block editor,
// whose hint panel shares the right column with the YAML editor/preview.
func TestBlockEditorHintAnimates(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	m := newHintModel(t, 60*time.Millisecond)
	updated, _ := m.Update(openItemMsg{Item: listItem{Key: "server"}})
	m = updated.(model)
	must.Equal(paneBlockEdit, m.mode, "expected paneBlockEdit")

	top := m.topBE()
	must.NotNil(top, "block editor must be open")
	shownH := top.hintH()
	must.Positive(shownH, "the block editor's hint panel starts open")
	editorH := top.editorH()

	be, cmd := top.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	must.NotNil(cmd, "toggling with animation on spawns a tick loop")
	must.True(be.hintAnim.active(), "a tween is in flight")
	is.False(be.showHint, "the flag flips immediately")
	is.True(be.hintVisible(), "the panel keeps its space while easing out")
	is.LessOrEqual(be.hintH(), shownH, "the panel is shrinking")

	deadline := time.Now().Add(2 * time.Second)
	for be.hintAnim.active() {
		must.False(time.Now().After(deadline), "animation never settled")
		be, cmd = be.Update(hintAnimTickMsg{block: true})
	}

	is.Nil(cmd, "the final frame stops rescheduling itself")
	is.Equal(0, be.hintH(), "the panel settles fully closed")
	is.False(be.hintVisible(), "the settled panel is gone")
	is.Greater(be.editorH(), editorH, "the YAML panel reclaims the freed rows")
}

// TestBlockEditorHintToggleInstantWithoutAnimation is the block editor's half
// of the no-timers-by-default contract.
func TestBlockEditorHintToggleInstantWithoutAnimation(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	m := newHintModel(t, 0)
	updated, _ := m.Update(openItemMsg{Item: listItem{Key: "server"}})
	m = updated.(model)
	top := m.topBE()
	must.NotNil(top, "block editor must be open")

	be, cmd := top.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	is.Nil(cmd, "no tick loop is spawned when animation is off")
	is.False(be.hintAnim.active(), "no tween is started when animation is off")
	is.Equal(0, be.hintH(), "the panel closes immediately")
}

// TestCurveDoesNotStallOrLurch is the smoothness contract, and the reason the
// curve is a quadratic rather than the tuios original's cubic.
//
// Perceived smoothness in a terminal is not about frame rate: heights are
// quantised to whole rows, so what reads as stutter is the height freezing for
// several consecutive frames and then lurching several rows at once. A cubic
// does exactly that - its flat ends repeat a height two or three times, then
// its steep middle skips rows through the middle.
//
// The spans are the panel heights hintTargetH settles on across the range of
// terminal sizes, from a 24-row terminal (floored at 5) to a 60-row one.
func TestCurveDoesNotStallOrLurch(t *testing.T) {
	is := assert.New(t)

	const dur = 180 * time.Millisecond
	frames := int(dur / animFrame)

	for _, span := range []struct {
		name     string
		from, to int
		maxJump  int
	}{
		{"short panel, opening", 0, 5, 2},
		{"typical panel, closing", 10, 0, 3},
		{"tall panel, opening", 0, 17, 4},
	} {
		start := time.Now()
		tw := tween{from: span.from, to: span.to, start: start, dur: dur}

		// A panel with fewer rows than there are frames must repeat heights: that
		// is quantisation, not stutter. The bound is what an even distribution
		// would give, so the assertion says "no worse than the arithmetic
		// forces" rather than a hand-picked number.
		maxStall := max(frames/abs(span.to-span.from), 1)

		prev, _ := tw.at(start)
		stall, worstStall, worstJump := 0, 0, 0
		for i := 1; i <= frames; i++ {
			v, _ := tw.at(start.Add(time.Duration(i) * animFrame))
			if v == prev {
				stall++
				worstStall = max(worstStall, stall)
			} else {
				stall = 0
				if jump := abs(v - prev); jump > worstJump {
					worstJump = jump
				}
			}
			prev = v
		}

		is.LessOrEqual(worstStall, maxStall, "%s: the height must not freeze beyond what quantisation forces", span.name)
		is.LessOrEqual(worstJump, span.maxJump, "%s: the height must not lurch", span.name)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
