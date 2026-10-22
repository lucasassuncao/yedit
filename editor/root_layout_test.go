package editor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/bezel/animation"
	"github.com/lucasassuncao/bezel/draw"
)

// TestHintPanelHMatchesTargetWhenIdle pins the invariant that keeps every
// pre-existing layout test valid: with no animation in flight the drawn height
// is exactly the height the static layout always computed.
func TestHintPanelHMatchesTargetWhenIdle(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, 0)

	is.True(m.showHint, "hints start shown when EnableHints is set")
	is.False(m.hintAnim.Active(), "no animation is in flight at rest")
	is.Equal(m.hintTargetH(), m.hintPanelH(), "idle height is the settled target")
	is.True(m.hintVisible(), "a shown panel is visible")
}

// TestZeroHeightHintPanelIsNotDrawn pins the specific condition behind the
// flicker, so a future change to hintVisible cannot quietly reintroduce it.
func TestZeroHeightHintPanelIsNotDrawn(t *testing.T) {
	is := assert.New(t)
	m := newHintModel(t, time.Second)

	// Park a tween at exactly zero while still marked in flight - the state the
	// tail frames of a hide transition sit in.
	m.showHint = false
	m.hintAnim = animation.Tween{From: 8, To: 0, Cur: 0, Start: time.Now(), Dur: time.Second}

	is.True(m.hintAnim.Active(), "the tween is still in flight")
	is.Equal(0, m.hintPanelH(), "the eased height has reached zero")
	is.False(m.hintVisible(), "a zero-height hint panel must not be drawn")
	preview := draw.InnerRect(m.relayoutHeights().sh.Rect("preview"))
	is.Equal(m.innerH(), preview.H, "the preview takes the whole column instead")
}
