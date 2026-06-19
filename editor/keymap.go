package editor

import tea "github.com/charmbracelet/bubbletea"

// listKeymap translates a KeyMsg into a ModelAction for the list view.
// Returns (action, true) for keys with semantic meaning; (nil, false) otherwise.
// Only called when the list is not in filtering mode.
// Keys that need a confirm check before acting (ctrl+r) are not handled here;
// handleListKey calls them directly.
func listKeymap(m model, key tea.KeyMsg) (ModelAction, bool) {
	switch key.String() {
	case "ctrl+u":
		return DocUndo{}, true
	case "ctrl+y":
		return DocRedo{}, true
	case "h":
		if m.cfg.EnableHints {
			return ToggleHints{}, true
		}
	}
	return nil, false
}
