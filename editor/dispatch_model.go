package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// dispatch applies a ModelAction and returns the updated model and any Cmd.
// All model-level mutations pass through here.
func (m model) dispatch(a ModelAction) (tea.Model, tea.Cmd) {
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
		m.showHint = !m.showHint
		m = m.relayout()
		return m, nil
	case ApplyDocPreset:
		// Show a confirmation dialog before replacing the entire document.
		// The actual replace is performed when confirmedDocPresetMsg is received.
		msg := fmt.Sprintf("Apply preset %q? This will replace the entire document — all unsaved changes will be lost.", act.Name)
		return m.showConfirmAlert("Apply document preset?", msg,
			func() tea.Msg { return confirmedDocPresetMsg(act) })
	default:
		panic(fmt.Sprintf("editor: unhandled ModelAction %T", a))
	}
}
