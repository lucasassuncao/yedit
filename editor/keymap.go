package editor

import tea "github.com/charmbracelet/bubbletea"

// blockKeymap translates a KeyMsg into an EditorAction for the block editor.
// Returns (action, true) for keys with semantic meaning; ({}, false) otherwise.
// Keys that need direct UI handling (tab, p, esc-with-dirty) return false.
func blockKeymap(top *blockEditState, key tea.KeyMsg) (EditorAction, bool) {
	switch key.String() {
	case "ctrl+s":
		return EditorAction{Model: CommitBlock{}}, true
	case "ctrl+u":
		if len(top.undoStack) > 0 {
			return EditorAction{Block: Undo{}}, true
		}
	case "ctrl+y":
		if len(top.redoStack) > 0 {
			return EditorAction{Block: Redo{}}, true
		}
	case "esc":
		if len(top.focus) > 0 {
			return EditorAction{Model: DrillOut{}}, true
		}
		if !top.dirty {
			return EditorAction{Model: DiscardBlock{}}, true
		}
		// dirty at root: caller shows the discard-changes confirmation dialog
	}
	return EditorAction{}, false
}

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
