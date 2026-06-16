package editor

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderStatusLine wraps text in style and constrains it to width - the shape
// shared by every feedback/legend line at the bottom of a screen.
func renderStatusLine(width int, style lipgloss.Style, text string) string {
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

// currentLegend returns the key/action legend line for the block editor's
// current panel and cursor state.
func (be blockEditState) currentLegend() string {
	if be.active != blockEditPanelTree {
		return legendSaveTail
	}
	parts := []string{keyNav, keyExpand}
	if be.cfg.Presets != nil && len(be.cfg.Presets.ListPresets(be.key)) > 0 {
		parts = append(parts, keyPreset)
	}
	if be.isCollectionNav() {
		parts = append(parts, keyEnterAdd, keyCtrlDDelete)
	} else {
		parts = append(parts, keyEnterAdd, keyCtrlDRemove)
	}
	parts = append(parts, keyCtrlUUndo, keyCtrlYRedo, keyTabPane, keyCtrlSSaveChg, keyEscBack)
	return strings.Join(parts, legendSep)
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
