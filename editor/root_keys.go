package editor

import tea "github.com/charmbracelet/bubbletea"

func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+s":
		mo, cmd := m.saveAll()
		return mo, cmd, true
	case "ctrl+l":
		mo, cmd := m.validateKeys()
		return mo, cmd, true
	}
	return m, nil, false
}

func (m model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.handleGlobalKey(msg); handled {
		return mo, cmd
	}

	if !m.list.IsFiltering() {
		switch msg.String() {
		case "tab":
			return m.togglePreviewPane()
		case "h":
			if m.cfg.Hints != nil {
				m.showHint = !m.showHint
				m.relayout()
				m.scrollPreviewToSelected()
			}
			return m, nil
		case "ctrl+u":
			return m.undo(), nil
		case "ctrl+y":
			return m.redo(), nil
		case "ctrl+r":
			return m.reload()
		case "esc", "ctrl+c":
			if m.doc.Dirty() {
				return m.showConfirmAlert("Quit without saving?",
					"Unsaved changes will be lost.", tea.Quit)
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.scrollPreviewToSelected()
	return m, cmd
}

func (m model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "esc":
		return m.togglePreviewPane()
	}
	// The preview is read-only; remaining keys only scroll the viewport.
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}
