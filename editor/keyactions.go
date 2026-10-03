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
	focusHintMsg       struct{}
	saveRequestedMsg   struct{}
	docUndoMsg         struct{}
	docRedoMsg         struct{}
	reloadRequestedMsg struct{}

	beBackMsg       struct{}
	beUndoMsg       struct{}
	beRedoMsg       struct{}
	beToggleHintMsg struct{}
	beFocusHintMsg  struct{}
	bePresetMsg     struct{}

	actionRequestedMsg struct{ key string }
)

// appActionKeys are the application's Config.Actions, shown next to save.
func (m model) appActionKeys() []shell.Action {
	out := make([]shell.Action, 0, len(m.cfg.Actions))
	for _, a := range m.cfg.Actions {
		out = append(out, send(a.Key, a.Help, actionRequestedMsg{key: a.Key}, secondRow))
	}
	return out
}

// shown is a key a component handles itself: the legend prints it, the shell
// leaves it to the list, tree or browser in focus.
func shown(keys, desc string, more ...string) shell.Action {
	opts := []shell.Option{shell.DisplayOnly()}
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

// shownLater is shown on the second row.
func shownLater(keys, desc string) shell.Action {
	return shell.Custom(keys, desc, nil, shell.DisplayOnly(), secondRow)
}

// quit asks first when there are unsaved changes.
func quit() shell.Action { return shell.Quit(shell.RunWith(shell.Send(quitRequestedMsg{}))) }

// hintKeys are the Hint/Example panel's two keys: focus it while it is shown,
// and show or hide it, worded for its current state.
func hintKeys(enabled, showing, toggle bool, focus, flip tea.Msg) []shell.Action {
	var out []shell.Action
	if enabled && showing {
		out = append(out, send("ctrl+h", "focus hint", focus))
	}
	if enabled && toggle {
		desc := "show hint"
		if showing {
			desc = "hide hint"
		}
		out = append(out, send("h", desc, flip))
	}
	return out
}

// docKeys are the root screen's second row, less what depends on the pane. The
// application's actions follow save, which they extend.
func docKeys(save, validate shell.Action, appActions []shell.Action) []shell.Action {
	out := append([]shell.Action{save}, appActions...)
	return append(out, send("ctrl+r", "reload", reloadRequestedMsg{}, secondRow),
		send("ctrl+u", "undo", docUndoMsg{}, secondRow), send("ctrl+y", "redo", docRedoMsg{}, secondRow), validate)
}

// actions is the root screen's key set for where the user is.
func (m model) actions() []shell.Action {
	save := send("ctrl+s", "save", saveRequestedMsg{}, secondRow)
	validate := send("ctrl+l", "validate", validateRequestedMsg{}, secondRow)
	hints := hintKeys(m.cfg.EnableHints, m.showHint, true, focusHintMsg{}, toggleHintsMsg{})
	switch {
	case m.mode == paneDocPreset:
		return append(browserActions(m.docPreset.PreviewFocus, "apply"), save, validate)
	case m.mode == panePreview:
		return []shell.Action{
			shell.ChangePane(), shell.Scroll(), send("esc", "back", previewBackMsg{}), quit(), shell.Help(),
			save, validate,
		}
	case m.mode == paneHint:
		// tab and esc both give the keys back to the pane they came from.
		out := []shell.Action{shell.ChangePane(shell.RunWith(shell.Send(focusHintMsg{}))), shell.Scroll()}
		out = append(out, hints...)
		out = append(out, send("esc", "back", focusHintMsg{}), quit(), shell.Help())
		return append(out, docKeys(save, validate, m.appActionKeys())...)
	case m.list.IsFiltering():
		// "?" and "q" are typed into the filter here.
		return []shell.Action{shell.Move(), shown("enter", "select"), shown("esc", "clear"), save, validate}
	}

	out := []shell.Action{shell.ChangePane(), shell.Move()}
	it := m.list.SelectedItem()
	switch {
	case it != nil && it.Unknown:
	case it != nil && it.Existing:
		out = append(out, shown("enter/→", "open", "enter", "right"))
	default:
		out = append(out, shown("enter/→", "add", "enter", "right"))
	}
	out = append(out, shown("/", "filter"))
	out = append(out, hints...)
	out = append(out, quit(), shell.Help())
	if len(presetItems(m.cfg.DocPresets, "")) > 0 {
		out = append(out, send("p", "presets", openDocPresetsMsg{}, secondRow))
	}
	appActions := m.appActionKeys()
	doc := docKeys(save, validate, appActions)
	head := 2 + len(appActions) // save, the app actions, reload
	out = append(out, doc[:head]...)
	if it != nil && it.Existing {
		out = append(out, shownLater("ctrl+d", "delete"))
	}
	return append(out, doc[head:]...)
}

// browserActions is a preset picker's legend; the browser handles every key.
func browserActions(previewFocus bool, enter string, extra ...shell.Action) []shell.Action {
	if previewFocus {
		return []shell.Action{shell.ChangePane(shell.DisplayOnly()), shell.Scroll(), shown("esc", "back")}
	}
	out := []shell.Action{shell.ChangePane(shell.DisplayOnly()), shell.Move(), shown("enter", enter)}
	return append(append(out, extra...), shown("esc", "cancel"))
}

// actions is the block editor's key set: "?" and the letter keys only where
// they are not typed into the YAML panel.
func (be blockEditState) actions() []shell.Action {
	if be.mode == modePresetBrowser {
		if be.isCollectionNav() {
			return browserActions(be.preset.PreviewFocus, "replace", shown("a", "append"))
		}
		return browserActions(be.preset.PreviewFocus, "apply")
	}

	typing := be.active == blockEditPanelYAML
	out := []shell.Action{shell.ChangePane()}
	switch be.active {
	case blockEditPanelTree:
		out = append(out, shell.Move(), shown("→/←", "expand", "right", "left"))
	case blockEditPanelHint:
		out = append(out, shell.Scroll())
	}
	out = append(out, hintKeys(be.cfg.EnableHints, be.showHint, !typing, beFocusHintMsg{}, beToggleHintMsg{})...)
	out = append(out, send("esc", "back", beBackMsg{}))
	if !typing {
		out = append(out, shell.Help())
	}
	if be.active == blockEditPanelTree {
		out = append(out, shownLater("enter", "add"))
		if be.cfg.BlockPresets != nil && len(be.cfg.BlockPresets.ListPresets(be.key)) > 0 {
			out = append(out, send("p", "presets", bePresetMsg{}, secondRow))
		}
	}
	out = append(out, send("ctrl+s", "save changes", commitRequestedMsg{}, secondRow))
	if be.active == blockEditPanelTree {
		remove := "remove"
		if be.isCollectionNav() {
			remove = "delete"
		}
		out = append(out, shownLater("ctrl+d", remove))
	}
	return append(out,
		send("ctrl+u", "undo", beUndoMsg{}, secondRow), send("ctrl+y", "redo", beRedoMsg{}, secondRow),
		send("ctrl+l", "validate", validateRequestedMsg{}, secondRow))
}
