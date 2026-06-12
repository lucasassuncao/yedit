package editor

import tea "github.com/charmbracelet/bubbletea"

// dispatch applies a ModelAction and returns the updated model and any Cmd.
// All model-level mutations pass through here.
func (m model) dispatch(a ModelAction) (tea.Model, tea.Cmd) {
	log := make([]ModelAction, len(m.actionLog)+1)
	copy(log, m.actionLog)
	log[len(m.actionLog)] = a
	m.actionLog = log
	switch act := a.(type) {
	case OpenBlock:
		return m.handleOpenItem(m.list.ItemByKey(act.Key))

	case CommitBlock:
		return m.saveAll()

	case DiscardBlock:
		m = m.enterList()
		return m, nil

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
	}
	return m, nil
}
