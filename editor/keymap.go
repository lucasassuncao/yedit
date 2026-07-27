package editor

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// listKeymap translates a KeyMsg into a ModelAction for the list view, and is
// only called outside filtering mode. Keys needing a confirm check before acting
// (ctrl+r) are handled directly by handleListKey instead.
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
