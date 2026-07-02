package editor

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// listKeymap translates a KeyMsg into a ModelAction for the list view.
// Returns (action, true) for keys with semantic meaning; (nil, false) otherwise.
// Only called when the list is not in filtering mode.
// Keys that need a confirm check before acting (ctrl+r) are not handled here;
// handleListKey calls them directly.
func listKeymap(m model, msg tea.KeyMsg) (ModelAction, bool) {
	switch {
	case key.Matches(msg, kbCtrlUUndo):
		return DocUndo{}, true
	case key.Matches(msg, kbCtrlYRedo):
		return DocRedo{}, true
	case key.Matches(msg, kbHint):
		if m.cfg.EnableHints {
			return ToggleHints{}, true
		}
	}
	return nil, false
}
