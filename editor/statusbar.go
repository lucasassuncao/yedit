package editor

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// renderStatusLine wraps text in style and constrains it to width. Used for
// feedback lines (errors, transient messages) — not for the legend.
func renderStatusLine(width int, style lipgloss.Style, text string) string {
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

// rowKeyMap is implemented by KeyMaps whose legend is split into fixed,
// semantically grouped lines (e.g. navigation vs. document-mutating actions)
// instead of a single ShortHelp list wrapped purely by width.
type rowKeyMap interface {
	help.KeyMap
	Rows() [][]key.Binding
}

// legendRow renders one row of bindings as styled "key desc" items joined by
// the separator, wrapping onto additional lines only if the row itself still
// exceeds maxWidth. Uses lipgloss.Width for ANSI-aware measurement.
func legendRow(h help.Model, bindings []key.Binding, maxWidth int) []string {
	sep := h.Styles.ShortSeparator.Render(h.ShortSeparator)
	sepW := lipgloss.Width(sep)

	var items []string
	var widths []int
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		str := h.Styles.ShortKey.Render(b.Help().Key)
		if b.Help().Desc != "" {
			str += " " + h.Styles.ShortDesc.Render(b.Help().Desc)
		}
		items = append(items, str)
		widths = append(widths, lipgloss.Width(str))
	}
	if len(items) == 0 {
		return nil
	}

	var lines []string
	var lineItems []string
	lineW := 0

	for i, item := range items {
		cost := widths[i]
		if len(lineItems) > 0 {
			cost += sepW
		}
		if len(lineItems) > 0 && lineW+cost > maxWidth {
			lines = append(lines, strings.Join(lineItems, sep))
			lineItems = []string{item}
			lineW = widths[i]
		} else {
			lineItems = append(lineItems, item)
			lineW += cost
		}
	}
	if len(lineItems) > 0 {
		lines = append(lines, strings.Join(lineItems, sep))
	}
	return lines
}

// renderLegend renders km's legend and returns the rendered string and the
// number of lines used. A KeyMap implementing rowKeyMap renders one line per
// group, in the group's fixed order, regardless of width (a group still
// wraps onto extra lines if it alone exceeds maxWidth). Any other KeyMap
// wraps its flat ShortHelp list onto new lines purely by width, as before.
func renderLegend(h help.Model, km help.KeyMap, maxWidth int) (string, int) {
	var lines []string
	if rk, ok := km.(rowKeyMap); ok {
		for _, row := range rk.Rows() {
			lines = append(lines, legendRow(h, row, maxWidth)...)
		}
	} else {
		lines = legendRow(h, km.ShortHelp(), maxWidth)
	}

	if len(lines) == 0 {
		return "", 1
	}
	return strings.Join(lines, "\n"), len(lines)
}

// renderHelpLine renders the legend with left padding, filling the full
// terminal width. Wraps onto multiple lines if the bindings exceed width-1.
func renderHelpLine(width int, h help.Model, km help.KeyMap) string {
	content, _ := renderLegend(h, km, width-1)
	return lipgloss.NewStyle().Width(width).Render(
		lipgloss.NewStyle().PaddingLeft(1).Render(content),
	)
}

// newHelpModel builds a help.Model styled to match the editor theme.
func newHelpModel(rt resolvedTheme) help.Model {
	h := help.New()
	h.ShowAll = false
	h.Styles.ShortKey = rt.hintKey
	h.Styles.ShortDesc = rt.hintDim
	h.Styles.ShortSeparator = rt.hintDim
	h.Styles.Ellipsis = rt.hintDim
	return h
}

// listKeyMapFor returns the correct help.KeyMap for the root list view based
// on current model state.
func listKeyMapFor(m model, previewFocused bool) help.KeyMap {
	if previewFocused {
		return listPreviewMap{}
	}
	if m.list.IsFiltering() {
		return listFilteringMap{}
	}
	hint := kbHint
	if m.showHint {
		hint = kbHintHide
	}
	if !m.cfg.EnableHints {
		hint.SetEnabled(false)
	}

	var km rowKeyMap
	it := m.list.SelectedItem()
	switch {
	case it != nil && it.Unknown:
		km = listUnknownMap{hint: hint}
	case it != nil && it.Existing:
		km = listExistingMap{hint: hint}
	default:
		km = listNewMap{hint: hint}
	}

	if m.cfg.DocPresets == nil {
		return km
	}
	// Insert kbTemplates into the navigation row, just before hint, so it
	// stays grouped with the other non-mutating keys.
	rows := km.Rows()
	nav := rows[0]
	extendedNav := make([]key.Binding, 0, len(nav)+1)
	extendedNav = append(extendedNav, nav[:len(nav)-1]...)
	extendedNav = append(extendedNav, kbTemplates, nav[len(nav)-1])
	extended := make([][]key.Binding, len(rows))
	extended[0] = extendedNav
	copy(extended[1:], rows[1:])
	return dynamicRows(extended)
}

// currentKeyMap returns the help.KeyMap for the block editor's current state.
// The tree panel's legend is grouped into two lines, split the same way as
// the root list legend (see listExistingMap.Rows): navigation/inspection
// (cursor and view only) vs. document actions (mutation/persistence).
func (be blockEditState) currentKeyMap() help.KeyMap {
	if be.active != blockEditPanelTree {
		return saveTailMap{}
	}
	nav := []key.Binding{kbNav, kbExpand}
	if be.cfg.BlockPresets != nil && len(be.cfg.BlockPresets.ListPresets(be.key)) > 0 {
		nav = append(nav, kbPreset)
	}
	nav = append(nav, kbTab, kbEscBack)

	var actions []key.Binding
	if be.isCollectionNav() {
		actions = []key.Binding{kbEnterAdd, kbCtrlDDelete}
	} else {
		actions = []key.Binding{kbEnterAdd, kbCtrlDRemove}
	}
	actions = append(actions, kbCtrlUUndo, kbCtrlYRedo, kbCtrlSSaveCh)

	return dynamicRows{nav, actions}
}

// feedbackLine picks the block editor's feedback line: an error takes
// priority, then the unsaved-changes notice, then any transient status message.
func (be blockEditState) feedbackLine() string {
	switch {
	case be.editorErr.kind != errNone:
		return renderStatusLine(be.width, be.theme.errorText, be.editorErr.message)
	case be.dirty:
		return renderStatusLine(be.width, be.theme.status, msgUncommittedChanges)
	case be.statusMsg != "":
		return renderStatusLine(be.width, be.theme.status, be.statusMsg)
	}
	return ""
}
