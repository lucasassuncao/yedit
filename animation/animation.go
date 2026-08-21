// Package animation provides the eased integer tween that drives the editor's
// panel transitions.
package animation

import (
	"math"
	"time"
)

// Frame is the interval between animation frames (~60fps). Terminal
// rows are coarse, so a tween resolves to only a handful of distinct heights;
// the frame rate exists to keep the easing curve smooth in time, not in space.
const Frame = 16 * time.Millisecond

// Tween interpolates an int between two values over a duration, easing in and
// out so the motion accelerates away from the start and settles into the end.
//
// The interpolation is adapted from the window minimize/restore animation in
// github.com/Gaurav-Gosain/tuios (MIT). That version drives four axes at once
// because its windows float in a compositor and travel toward a dock; yedit's
// hint panel is stacked in the right column and can only change height, so this
// is reduced to a single axis. The curve is not the original's - see
// easeInOutQuad for why a gentler one suits cell-quantised output.
//
// The zero value is inactive: At reports the tween as finished and callers fall
// back to the panel's settled height.
type Tween struct {
	From, To int
	Cur      int // value sampled by the last advance; the height the frame draws
	Start    time.Time
	Dur      time.Duration
}

// Active reports whether t is currently in flight.
func (t Tween) Active() bool { return t.Dur > 0 }

// Advance samples the curve at now and latches the result into Cur, reporting
// whether the tween has finished.
//
// The renderer reads Cur rather than calling At itself, so every height derived
// during one frame comes from a single sample. Sampling per read instead would
// let the clock cross a step between two reads of the same frame and lay the
// hint panel out one row apart from the space the preview left for it.
func (t *Tween) Advance(now time.Time) bool {
	v, done := t.At(now)
	t.Cur = v
	return done
}

// At returns the interpolated value at now, and whether the tween has finished.
// A finished tween reports its end value exactly, so the panel always lands on
// the height the static layout would have chosen.
func (t Tween) At(now time.Time) (int, bool) {
	if t.Dur <= 0 {
		return t.To, true
	}
	p := float64(now.Sub(t.Start)) / float64(t.Dur)
	if p >= 1 {
		return t.To, true
	}
	if p < 0 {
		p = 0
	}
	return t.From + int(math.Round(float64(t.To-t.From)*easeInOutQuad(p))), false
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

// New builds the tween carrying a panel from its current on-screen
// height to target over dur. It returns the zero tween when animation is
// disabled (dur <= 0) or there is no distance to cover, which makes the caller
// fall through to an instant toggle.
func New(from, target int, dur time.Duration) Tween {
	if dur <= 0 || from == target {
		return Tween{}
	}
	return Tween{From: from, To: target, Cur: from, Start: time.Now(), Dur: dur}
}
