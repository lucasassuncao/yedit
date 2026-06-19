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

// renderLegend renders km's short bindings, wrapping onto new lines when they
// would exceed maxWidth. Returns the rendered string and the number of lines
// used. Uses lipgloss.Width for ANSI-aware measurement.
func renderLegend(h help.Model, km help.KeyMap, maxWidth int) (string, int) {
	sep := h.Styles.ShortSeparator.Render(h.ShortSeparator)
	sepW := lipgloss.Width(sep)

	var items []string
	var widths []int
	for _, b := range km.ShortHelp() {
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
		return "", 1
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
	if it := m.list.SelectedItem(); it != nil {
		if it.Unknown {
			return listUnknownMap{hint: hint}
		}
		if it.Existing {
			return listExistingMap{hint: hint}
		}
	}
	return listNewMap{hint: hint}
}

// currentKeyMap returns the help.KeyMap for the block editor's current state.
func (be blockEditState) currentKeyMap() help.KeyMap {
	if be.active != blockEditPanelTree {
		return saveTailMap{}
	}
	parts := []key.Binding{kbNav, kbExpand}
	if be.cfg.Presets != nil && len(be.cfg.Presets.ListPresets(be.key)) > 0 {
		parts = append(parts, kbPreset)
	}
	if be.isCollectionNav() {
		parts = append(parts, kbEnterAdd, kbCtrlDDelete)
	} else {
		parts = append(parts, kbEnterAdd, kbCtrlDRemove)
	}
	parts = append(parts, kbCtrlUUndo, kbCtrlYRedo, kbTab, kbCtrlSSaveCh, kbEscBack)
	return dynamicKeyMap(parts)
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
