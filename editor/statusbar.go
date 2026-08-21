package editor

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/lucasassuncao/yedit/legend"

	"github.com/lucasassuncao/yedit/keys"
)

const msgUncommittedChanges = "Uncommitted changes - ctrl+s to commit"

// listKeyMapFor returns the correct help.KeyMap for the root list view based
// on current model state.
func listKeyMapFor(m model, previewFocused bool) help.KeyMap {
	if previewFocused {
		return legend.ListPreview{}
	}
	if m.list.IsFiltering() {
		return legend.ListFiltering{}
	}
	hint := keys.Hint
	if m.showHint {
		hint = keys.HintHide
	}
	if !m.cfg.EnableHints {
		hint.SetEnabled(false)
	}

	var km legend.RowKeyMap
	it := m.list.SelectedItem()
	switch {
	case it != nil && it.Unknown:
		km = legend.ListUnknown{Hint: hint}
	case it != nil && it.Existing:
		km = legend.ListExisting{Hint: hint}
	default:
		km = legend.ListNew{Hint: hint}
	}

	if m.cfg.DocPresets == nil {
		return km
	}
	// Insert keys.Templates into the navigation row, just before hint, so it
	// stays grouped with the other non-mutating keys.
	rows := km.Rows()
	nav := rows[0]
	extendedNav := make([]key.Binding, 0, len(nav)+1)
	extendedNav = append(extendedNav, nav[:len(nav)-1]...)
	extendedNav = append(extendedNav, keys.Templates, nav[len(nav)-1])
	extended := make([][]key.Binding, len(rows))
	extended[0] = extendedNav
	copy(extended[1:], rows[1:])
	return legend.DynamicRows(extended)
}

// currentKeyMap returns the help.KeyMap for the block editor's current state.
// The tree panel's legend is grouped into two lines, split the same way as
// the root list legend (see legend.ListExisting.Rows): navigation/inspection
// (cursor and view only) vs. document actions (mutation/persistence).
func (be blockEditState) currentKeyMap() help.KeyMap {
	if be.active != blockEditPanelTree {
		return legend.SaveTail{}
	}
	nav := []key.Binding{keys.Nav, keys.Expand}
	if be.cfg.BlockPresets != nil && len(be.cfg.BlockPresets.ListPresets(be.key)) > 0 {
		nav = append(nav, keys.Preset)
	}
	if be.cfg.EnableHints {
		hint := keys.Hint
		if be.showHint {
			hint = keys.HintHide
		}
		nav = append(nav, hint)
	}
	nav = append(nav, keys.Tab, keys.EscBack)

	var actions []key.Binding
	if be.isCollectionNav() {
		actions = []key.Binding{keys.EnterAdd, keys.CtrlDDelete}
	} else {
		actions = []key.Binding{keys.EnterAdd, keys.CtrlDRemove}
	}
	actions = append(actions, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlSSaveCh)

	return legend.DynamicRows{nav, actions}
}

// feedbackLine picks the block editor's feedback line: an error takes
// priority, then the unsaved-changes notice, then any transient status message.
func (be blockEditState) feedbackLine() string {
	switch {
	case be.editorErr.kind != errNone:
		return legend.StatusLine(be.width, be.theme.ErrorText, be.editorErr.message)
	case be.dirty:
		return legend.StatusLine(be.width, be.theme.Status, msgUncommittedChanges)
	case be.statusMsg != "":
		return legend.StatusLine(be.width, be.theme.Status, be.statusMsg)
	}
	return ""
}
