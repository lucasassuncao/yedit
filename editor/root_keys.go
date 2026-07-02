package editor

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, kbCtrlSSave):
		mo, cmd := m.dispatch(CommitBlock{})
		return mo, cmd, true
	case key.Matches(msg, kbCtrlLValid):
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
		switch {
		case key.Matches(msg, kbTabPreview):
			return m.togglePreviewPane()
		case key.Matches(msg, kbCtrlRReload):
			return m.reload()
		case key.Matches(msg, kbTemplates):
			if pb, ok := newPresetBrowser(m.cfg.DocPresets, "", ""); ok {
				return m.enterDocPreset(pb), nil
			}
		case key.Matches(msg, kbEsc):
			// ctrl+c is handled for every mode in handleModeUpdate.
			return m.quitOrConfirm()
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
	if key.Matches(msg, kbTabEscList) {
		return m.togglePreviewPane()
	}
	// The preview is read-only; remaining keys only scroll the viewport.
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}
