package editor

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// dispatch applies a ModelAction. Every model-level mutation passes through it.
func (m model) dispatch(a ModelAction) (tea.Model, tea.Cmd) {
	if m.cfg.Trace.OnModelAction != nil {
		m.cfg.Trace.OnModelAction(a)
	}
	switch act := a.(type) {
	case OpenBlock:
		return m.handleOpenItem(m.list.ItemByKey(act.Key))

	case CommitBlock:
		return m.saveAll()

	case DeleteBlock:
		return m.handleDelete(act.Key)

	case DrillIn:
		return m.handleOpenChild(openChildMsg{
			key:     act.Key,
			defs:    act.Defs,
			kind:    act.Kind,
			relSegs: act.RelSegs,
		})

	case DrillOut:
		return m.handleDrillOut()

	case DocUndo:
		return m.undo()

	case DocRedo:
		return m.redo()

	case Save:
		return m.execSave()

	case Reload:
		return m.execReload()

	case ToggleHints:
		// Sample the on-screen height before flipping the flag: mid-flight it is
		// neither 0 nor the settled target.
		from := m.hintPanelH()
		m.showHint = !m.showHint
		m, tick := m.startHintAnim(from)
		m = m.relayout()
		if tick {
			return m, hintAnimTick(false)
		}
		return m, nil

	case ApplyDocPreset:
		// Confirm before replacing the whole document; the replace happens on
		// confirmedDocPresetMsg.
		msg := fmt.Sprintf("Apply preset %q? This will replace the entire document - all unsaved changes will be lost.", act.Name)
		return m.showConfirmAlert("Apply document preset?", msg,
			func() tea.Msg { return confirmedDocPresetMsg(act) })

	default:
		panic(fmt.Sprintf("editor: unhandled ModelAction %T", a))
	}
}
