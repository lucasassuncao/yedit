package editor

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootHintPanelToggle verifies that EnableHints opens the panel on start,
// "h" toggles it off and back on, and the field metadata is visible.
func TestRootHintPanelToggle(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	m, err := newModel(Config{
		Path:        filepath.Join(t.TempDir(), "hint.yaml"),
		Schema:      &sizeProbeConfig{},
		EnableHints: true,
		Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
			if block == "server" && fieldPath == "" {
				return FieldMeta{Type: "object"}
			}
			return FieldMeta{}
		}),
	})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// EnableHints: true → panel visible on start.
	must.True(m.showHint, "EnableHints: true should show the hint panel on start")
	view := m.viewContent()
	is.Contains(view, "Hint/Example", "view should show the Hint/Example panel when EnableHints is true")
	is.Contains(view, "object", "hint should show the selected field's type (server → object)")

	// "h" hides the panel.
	m, _ = pressAction(t, m, tea.KeyPressMsg{Text: "h", Code: 'h'})
	must.False(m.showHint, "pressing h should hide the hint panel")
	is.NotContains(m.viewContent(), "Hint/Example", "hint panel should be hidden after pressing h")

	// "h" again shows it.
	m, _ = pressAction(t, m, tea.KeyPressMsg{Text: "h", Code: 'h'})
	must.True(m.showHint, "pressing h again should re-enable the hint panel")
	is.Contains(m.viewContent(), "Hint/Example", "hint panel should be visible after toggling back on")
}

// Without a MetadataSource the panel says why it has nothing to show.
func TestSelectedHintWithoutMetadata(t *testing.T) {
	m, err := newModel(Config{Path: filepath.Join(t.TempDir(), "hint.yaml"), Schema: &sizeProbeConfig{}})
	require.NoError(t, err)
	require.Contains(t, ansi.Strip(m.selectedHint()), "Config.Metadata is not set")
}

// With the keys, ↓ scrolls a hint longer than the panel, and stops at its end.
func TestTheFocusedHintPanelScrolls(t *testing.T) {
	long := strings.Repeat("line\n", 60)
	m, err := newModel(Config{
		Path:        filepath.Join(t.TempDir(), "probe.yaml"),
		Schema:      &hintProbeConfig{},
		EnableHints: true,
		Metadata:    MetadataFunc(func(string, string) FieldMeta { return FieldMeta{Example: long} }),
	})
	require.NoError(t, err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model).toggleHintFocus()
	require.Equal(t, paneHint, m.mode)

	top := m.hintView()
	for range 200 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(model)
	}
	require.NotEqual(t, top, m.hintView(), "↓ did not scroll the hint")
	require.Equal(t, m.maxHintScroll(), m.hintScroll, "the scroll runs past the end of the hint")
}

// ctrl+h gives the keys to the Hint/Example panel and takes them back, as in
// the block editor; esc and hiding the panel give them back too.
func TestCtrlHFocusesTheHintPanelOnTheMainScreen(t *testing.T) {
	press := func(m model, k tea.KeyPressMsg) model {
		updated, cmd := m.Update(k)
		m = updated.(model)
		if cmd != nil {
			if msg := cmd(); msg != nil {
				updated, _ = m.Update(msg)
				m = updated.(model)
			}
		}
		return m
	}
	ctrlH := tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}

	m := press(newHintModel(t, 0), ctrlH)
	require.Equal(t, paneHint, m.mode)
	require.Equal(t, "hint", m.rootPane())
	require.Equal(t, paneList, press(m, ctrlH).mode, "ctrl+h again gives the keys back")
	require.Equal(t, paneList, press(m, tea.KeyPressMsg{Code: tea.KeyEscape}).mode, "esc gives them back")

	hidden := press(m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.False(t, hidden.showHint)
	require.Equal(t, paneList, hidden.mode, "a hidden panel cannot keep the keys")
}
