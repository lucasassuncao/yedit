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
		newDoc, err := m.doc.ReplaceRaw([]byte(act.Content))
		if err != nil {
			return m.withStatus(fmt.Sprintf("Failed to apply preset %q: %v", act.Name, err))
		}
		m.doc = newDoc
		m = m.syncView()
		return m.withStatus(fmt.Sprintf("Applied preset %q — ctrl+s to save.", act.Name))
	default:
		panic(fmt.Sprintf("editor: unhandled ModelAction %T", a))
	}
}
