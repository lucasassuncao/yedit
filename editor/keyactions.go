package editor

import (
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/shell"
)

// Each screen declares its keys once, as bezel actions: the shell prints them
// in the legend and runs them. An action on a value model sends one of these
// messages, handled in Update, instead of closing over a stale copy.
type (
	previewBackMsg     struct{}
	quitRequestedMsg   struct{}
	openDocPresetsMsg  struct{}
	toggleHintsMsg     struct{}
	saveRequestedMsg   struct{}
	docUndoMsg         struct{}
	docRedoMsg         struct{}
	reloadRequestedMsg struct{}

	beBackMsg       struct{}
	beUndoMsg       struct{}
	beRedoMsg       struct{}
	beToggleHintMsg struct{}
	bePresetMsg     struct{}
	bePickPathMsg   struct{}
	bePickValueMsg  struct{}

	actionRequestedMsg struct{ key string }
)

// appActionKeys are the application's Config.Actions, shown next to save.
func (m model) appActionKeys() []shell.Action {
	out := make([]shell.Action, 0, len(m.cfg.Actions))
	for _, a := range m.cfg.Actions {
		out = append(out, send(a.Key, a.Help, actionRequestedMsg{key: a.Key}, secondRow, editing))
	}
	return out
}

// shown is a key a component handles itself: the legend prints it, the shell
// leaves it to the list, tree or browser in focus.
func shown(keys, desc string, more ...string) shell.Action {
	opts := []shell.Option{shell.DisplayOnly(), nav}
	if len(more) > 0 {
		opts = append(opts, shell.WithKey(more...))
	}
	return shell.Custom(keys, desc, nil, opts...)
}

func send(keys, desc string, msg tea.Msg, opts ...shell.Option) shell.Action {
	return shell.Custom(keys, desc, shell.Send(msg), opts...)
}

// secondRow starts the legend's second row, when it has two: the first gets
// around, the hint panel included, help last; the second changes the document.
var secondRow = shell.Group(1)

// nav, editing and view head the help's sections: getting around, what
// changes the document, and what changes the screen.
var (
	nav     = shell.Section("navigation")
	editing = shell.Section("editing")
	view    = shell.Section("view")
)

// shownLater is shown on the second row.
func shownLater(keys, desc string) shell.Action {
	return shell.Custom(keys, desc, nil, shell.DisplayOnly(), secondRow, editing)
}

// quit asks first when there are unsaved changes.
func quit() shell.Action { return shell.Quit(shell.RunWith(shell.Send(quitRequestedMsg{})), nav) }

// quitKeys is q, on the main screen only: a file tab closes with esc and
// leaves quitting to the main screen.
func (m model) quitKeys() []shell.Action {
	if m.inFileTab {
		return nil
	}
	return []shell.Action{quit()}
}

// hintKeys is h, which shows or hides the Hint/Example panel, worded for its
// state. The help lists it; the legend leaves it out.
func hintKeys(enabled, showing bool, flip tea.Msg) []shell.Action {
	if !enabled {
		return nil
	}
	desc := "show hint"
	if showing {
		desc = "hide hint"
	}
	return []shell.Action{send("h", desc, flip, shell.HelpOnly(), view)}
}

// docKeys are the root screen's second row, less what depends on the pane. The
// application's actions follow save, which they extend.
func docKeys(save, validate shell.Action, appActions []shell.Action) []shell.Action {
	out := append([]shell.Action{save}, appActions...)
	return append(out, send("ctrl+r", "reload", reloadRequestedMsg{}, secondRow, editing, shell.HelpOnly()),
		send("ctrl+u", "undo", docUndoMsg{}, secondRow, editing), send("ctrl+y", "redo", docRedoMsg{}, secondRow, editing), validate)
}

// withTabKey puts ctrl+t after lead when other screens are beside this one:
// the tabs handle it before any screen sees it, whatever has the keys.
func withTabKey(tabbed bool, lead, keys []shell.Action) []shell.Action {
	out := append([]shell.Action(nil), lead...)
	if tabbed {
		out = append(out, shown(tabKey, "change tab"))
	}
	return append(out, keys...)
}

// actions is the root screen's key set for where the user is.
func (m model) actions() []shell.Action {
	lead, keys := m.screenActions()
	return withTabKey(m.tabbed, lead, keys)
}

// screenActions is the keys for where the user is, and lead, the ones before
// ctrl+t: esc, which closes a file tab from its block list.
func (m model) screenActions() (lead, keys []shell.Action) {
	save := send("ctrl+s", "save", saveRequestedMsg{}, secondRow, editing)
	validate := send("ctrl+l", "validate", validateRequestedMsg{}, secondRow, editing)
	hints := hintKeys(m.cfg.EnableHints, m.showHint, toggleHintsMsg{})
	switch base := m.basePane(); {
	case base == paneDocPreset:
		return nil, append(browserActions(m.docPreset.PreviewFocused(), "apply"), save, validate)
	case base == panePreview:
		out := []shell.Action{shell.ChangePane(nav), shell.Scroll(nav), send("esc", "back", previewBackMsg{}, nav)}
		return nil, append(append(out, m.quitKeys()...), shell.Help(), save, validate, shell.Resize(shell.HelpOnly(), view), shell.ResetSizes(shell.HelpOnly(), view))
	case m.list.Filtering():
		// "?" and "q" are typed into the filter here.
		return nil, []shell.Action{shell.Move(nav), shown("enter", "select"), shown("esc", "clear"), save, validate}
	}

	if m.inFileTab {
		lead = []shell.Action{send("esc", "close tab", closeFileRequestedMsg{}, nav)}
	}
	out := []shell.Action{shell.ChangePane(nav), shell.Move(nav)}
	it := m.list.SelectedItem()
	switch {
	case it != nil && it.Unknown:
	case it != nil && it.Existing:
		out = append(out, shown("enter/→", "open", "enter", "right"))
	default:
		out = append(out, shown("enter/→", "add", "enter", "right"))
	}
	out = append(out, shown("/", "filter"))
	out = append(append(out, m.quitKeys()...), shell.Help())
	if len(presetItems(m.cfg.DocPresets, "")) > 0 {
		out = append(out, send("p", "presets", openDocPresetsMsg{}, secondRow, editing))
	}
	appActions := m.appActionKeys()
	doc := docKeys(save, validate, appActions)
	head := 2 + len(appActions) // save, the app actions, reload
	out = append(out, doc[:head]...)
	if it != nil && it.Existing {
		out = append(out, shownLater("ctrl+d", "delete"))
	}
	out = append(out, doc[head:]...)
	out = append(out, hints...)
	return lead, append(out, shell.Resize(shell.HelpOnly(), view), shell.ResetSizes(shell.HelpOnly(), view))
}

// browserActions is a preset picker's legend; the browser handles every key.
func browserActions(previewFocus bool, enter string, extra ...shell.Action) []shell.Action {
	if previewFocus {
		return []shell.Action{shell.ChangePane(shell.DisplayOnly(), nav), shell.Scroll(nav), shown("esc", "back")}
	}
	out := []shell.Action{shell.ChangePane(shell.DisplayOnly(), nav), shell.Move(nav), shell.Custom("enter", enter, nil, shell.DisplayOnly(), editing)}
	return append(append(out, extra...), shown("esc", "cancel"))
}

// actions is the block editor's key set: "?" and the letter keys only where
// they are not typed into the YAML panel.
func (be blockEditState) actions() []shell.Action {
	return withTabKey(be.tabbed, nil, be.panelActions())
}

func (be blockEditState) panelActions() []shell.Action {
	if be.mode == modePresetBrowser {
		if be.isCollectionNav() {
			return browserActions(be.preset.PreviewFocused(), "replace", shell.Custom("a", "append", nil, shell.DisplayOnly(), editing))
		}
		return browserActions(be.preset.PreviewFocused(), "apply")
	}

	typing := be.active == blockEditPanelYAML
	out := []shell.Action{shell.ChangePane(nav)}
	if be.active == blockEditPanelTree {
		out = append(out, shell.Move(nav), shown("→/←", "expand", "right", "left"))
	}
	out = append(out, send("esc", "back", beBackMsg{}, nav))
	if !typing {
		out = append(out, shell.Help())
	}
	if be.active == blockEditPanelTree {
		enter := "add"
		if be.openableEntry() != "" {
			enter = "open file"
		}
		out = append(out, shownLater("enter", enter))
		if be.cfg.BlockPresets != nil && len(be.cfg.BlockPresets.ListPresets(be.key)) > 0 {
			out = append(out, send("p", "presets", bePresetMsg{}, secondRow, editing))
		}
	}
	if _, ok := be.pathField(); ok {
		out = append(out, send("ctrl+o", "pick file", bePickPathMsg{}, secondRow, editing))
	} else if _, ok := be.valueChoice(); ok {
		out = append(out, send("ctrl+o", "pick value", bePickValueMsg{}, secondRow, editing))
	}
	out = append(out, send("ctrl+s", "save changes", commitRequestedMsg{}, secondRow, editing))
	if be.active == blockEditPanelTree {
		remove := "remove"
		if be.isCollectionNav() {
			remove = "delete"
		}
		out = append(out, shownLater("ctrl+d", remove))
	}
	out = append(out,
		send("ctrl+u", "undo", beUndoMsg{}, secondRow, editing), send("ctrl+y", "redo", beRedoMsg{}, secondRow, editing),
		send("ctrl+l", "validate", validateRequestedMsg{}, secondRow, editing))
	if !typing {
		// The YAML panel types h and "=", and selects words with ctrl+shift+arrows.
		out = append(out, hintKeys(be.cfg.EnableHints, be.showHint, beToggleHintMsg{})...)
		out = append(out, shell.Resize(shell.HelpOnly(), view), shell.ResetSizes(shell.HelpOnly(), view))
	}
	return out
}
