package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+s":
		mo, cmd := m.dispatch(CommitBlock{})
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
		case "ctrl+r":
			return m.reload()
		case "ctrl+p":
			if pb, ok := newPresetBrowser(m.cfg.DocPresets, "", ""); ok {
				return m.enterDocPreset(pb), nil
			}
		case "esc", "ctrl+c":
			if m.doc.Dirty() {
				return m.showConfirmAlert("Quit without saving?",
					"Unsaved changes will be lost.", tea.Quit)
			}
			return m, tea.Quit
		}
		if ma, ok := listKeymap(m, msg); ok {
			return m.dispatch(ma)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m = m.scrollPreviewToSelected()
	return m, cmd
}

func (m model) handleDocPresetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.handleGlobalKey(msg); handled {
		return mo, cmd
	}
	pb, action, name := m.docPreset.Update(msg, false)
	m.docPreset = pb
	switch action {
	case presetDismissed:
		return m.enterList(), nil
	case presetApplied:
		y, err := m.cfg.DocPresets.PresetYAML("", name)
		if err != nil {
			return m.withStatus(fmt.Sprintf("preset error: %v", err))
		}
		m = m.enterList()
		return m.dispatch(ApplyDocPreset{Name: name, Content: y})
	}
	return m, nil
}

func (m model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global shortcuts (save, validate) are available in every mode.
	if mo, cmd, handled := m.handleGlobalKey(msg); handled {
		return mo, cmd
	}
	switch msg.String() {
	case "tab", "esc":
		return m.togglePreviewPane()
	}
	// The preview is read-only; remaining keys only scroll the viewport.
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}
