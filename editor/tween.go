package editor

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/animation"
)

// hintAnimTickMsg advances an in-flight hint panel animation. block marks a
// tick belonging to the active block editor rather than the root list, so the
// root Update can route it to the right owner.
type hintAnimTickMsg struct{ block bool }

// hintAnimTick schedules the next animation frame. The loop is self-cancelling:
// the handler stops rescheduling once the tween finishes, so an idle editor
// emits no ticks at all.
func hintAnimTick(block bool) tea.Cmd {
	return tea.Tick(animation.Frame, func(time.Time) tea.Msg { return hintAnimTickMsg{block: block} })
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

	if !m.hintAnim.Active() {
		return m, nil
	}
	if m.hintAnim.Advance(time.Now()) {
		m.hintAnim = animation.Tween{}
		// Settled: run the full relayout once so the preview renderer and its
		// content are rebuilt for the final height.
		return m.relayout(), nil
	}
	return m.relayoutHeights(), hintAnimTick(false)
}

// advanceHintAnim is the block editor's counterpart to handleHintAnimTick.
func (be blockEditState) advanceHintAnim() (blockEditState, tea.Cmd) {
	if !be.hintAnim.Active() {
		return be, nil
	}
	done := be.hintAnim.Advance(time.Now())
	if done {
		be.hintAnim = animation.Tween{}
	}
	// editorH shrinks and grows with the hint panel, and the textarea keeps its
	// height until told otherwise, so it has to be resized on every frame.
	be.yamlEditor.SetHeight(be.editorH() - 1)
	if done {
		return be, nil
	}
	return be, hintAnimTick(true)
}
