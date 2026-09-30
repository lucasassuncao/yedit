package editor

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/browser"
	"github.com/lucasassuncao/yedit/keys"
)

// handleGlobalKey keeps save and validate live under an alert, where the
// shell holds the keys and runs no action.
func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, keys.CtrlSSave):
		mo, cmd := m.dispatch(CommitBlock{})
		return mo, cmd, true
	case key.Matches(msg, keys.CtrlLValid):
		mo, cmd := m.validateKeys()
		return mo, cmd, true
	}
	return m, nil, false
}

// runAction offers msg to the shell with this screen's actions. A Help that
// opened its overlay puts the editor in alert mode, which closes it.
func (m model) runAction(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, false
	}
	sh, handled, cmd := m.sh.SetFocus(m.rootPane()).SetActions(m.actions()...).Update(km)
	if !handled {
		return m, nil, false
	}
	m.sh = sh
	if sh.HasOverlay() {
		m.mode = paneAlert
	}
	return m, cmd, true
}

func (m model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.runAction(msg); handled {
		return mo, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m = m.scrollPreviewToSelected()
	return m, cmd
}

func (m model) handleDocPresetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.runAction(msg); handled {
		return mo, cmd
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	var action browser.Action
	m.docPreset, action = m.docPreset.Update(km)
	switch action {
	case browser.Dismissed:
		return m.enterList().relayout(), nil
	case browser.Chosen:
		name := m.docPreset.Selected().Label
		y, err := m.cfg.DocPresets.PresetYAML("", name)
		if err != nil {
			return m.withStatus(fmt.Sprintf("preset error: %v", err))
		}
		m = m.enterList().relayout()
		return m.dispatch(ApplyDocPreset{Name: name, Content: y})
	}
	return m, nil
}

func (m model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.runAction(msg); handled {
		return mo, cmd
	}
	// The preview is read-only; remaining keys only scroll the viewport.
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

// openDocPresets switches to the document preset picker, when there are any.
func (m model) openDocPresets() (tea.Model, tea.Cmd) {
	if items := presetItems(m.cfg.DocPresets, ""); len(items) > 0 {
		return m.enterDocPreset(browser.New(items, "")), nil
	}
	return m, nil
}
