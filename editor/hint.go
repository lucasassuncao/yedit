package editor

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/hint"
	"github.com/lucasassuncao/yedit/keys"
)

// toggleHintFocus gives the keys to the Hint/Example panel to scroll it, or
// back to the pane they came from, as ctrl+h does in the block editor.
func (m model) toggleHintFocus() model {
	if m.mode == paneHint {
		m.mode = m.hintFrom
		return m
	}
	if !m.hintVisible() || (m.mode != paneList && m.mode != panePreview) {
		return m
	}
	m.hintFrom, m.mode, m.hintScroll = m.mode, paneHint, 0
	return m
}

// toggleHints shows or hides the panel; hiding it hands back the keys it had.
func (m model) toggleHints() (tea.Model, tea.Cmd) {
	mo, cmd := m.dispatch(ToggleHints{})
	if next, ok := mo.(model); ok && !next.showHint && next.mode == paneHint {
		next.mode = next.hintFrom
		return next, cmd
	}
	return mo, cmd
}

// handleHintKey scrolls the focused panel; the actions run first.
func (m model) handleHintKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.runAction(msg); handled {
		return mo, cmd
	}
	switch {
	case key.Matches(msg, keys.Up):
		m.hintScroll = max(m.hintScroll-1, 0)
	case key.Matches(msg, keys.Down):
		m.hintScroll = min(m.hintScroll+1, m.maxHintScroll())
	}
	return m, nil
}

// maxHintScroll is bound by the hint's length, so its tail stays reachable.
func (m model) maxHintScroll() int {
	lines := strings.Count(strings.TrimSuffix(m.selectedHint(), "\n"), "\n") + 1
	return max(lines-m.hintPanelH(), 0)
}

// hintView is the panel's body, scrolled while it has the keys.
func (m model) hintView() string {
	content := m.selectedHint()
	if m.mode != paneHint {
		return content
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	from := min(m.hintScroll, m.maxHintScroll())
	return strings.Join(lines[from:min(from+m.hintPanelH(), len(lines))], "\n")
}

// selectedHint renders the Hint/Example panel body for the selected list item.
// All display data comes from MetadataSource.
func (m model) selectedHint() string {
	if m.cfg.Metadata == nil {
		return m.theme.Muted.Render("  Config.Metadata is not set - no metadata source configured")
	}
	it := m.list.SelectedItem()
	if it == nil || it.Separator {
		return m.theme.Muted.Render("  select a field to see hints")
	}
	if it.Unknown {
		return m.theme.Muted.Render("  unknown key - not in the schema")
	}
	def := fieldDefByName(m.schemaTree, it.Key)
	if def.YAMLName == "" {
		def.YAMLName = it.Key
	}
	meta := m.cfg.Metadata.FieldMeta(it.Key, "")
	ex := meta.Example
	if ex == "" && meta.Multiline {
		ex = it.Key + ": |\n  line 1\n  line 2\n"
	}
	if out := hint.Render(m.theme, meta, ex); out != "" {
		return out
	}
	return m.theme.Muted.Render("  no metadata declared for this field")
}
