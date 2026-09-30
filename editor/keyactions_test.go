package editor

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/shell"
)

// pressAction presses a key bound to an action and, as the runtime would,
// delivers the message the action sent. It returns what that message caused.
func pressAction(t *testing.T, m model, k tea.KeyPressMsg) (model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(k)
	require.NotNil(t, cmd, "%s runs an action", k)
	updated, cmd = updated.Update(cmd())
	return updated.(model), cmd
}

// pressBEAction is pressAction for a block editor driven directly.
func pressBEAction(t *testing.T, be blockEditState, k tea.KeyPressMsg) (blockEditState, tea.Cmd) {
	t.Helper()
	be, cmd := be.Update(k)
	require.NotNil(t, cmd, "%s runs an action", k)
	return be.Update(cmd())
}

func TestEveryScreenHasOneActionPerKey(t *testing.T) {
	m := newHintModel(t, 0)
	require.NoError(t, shell.Check(m.actions()...), "list")
	m.mode = panePreview
	require.NoError(t, shell.Check(m.actions()...), "preview")

	be := newServerBlock(t)
	for _, p := range []blockEditPanel{blockEditPanelTree, blockEditPanelYAML, blockEditPanelHint} {
		be.active = p
		require.NoError(t, shell.Check(be.actions()...), "block editor panel %d", p)
	}
}

// Tab walks fields and the YAML editor through the shell's ChangePane: the
// editor takes the cursor on the way in, and the hint is never a stop.
func TestBlockEditTabCyclesPanesAndSkipsHint(t *testing.T) {
	must := require.New(t)
	be := newBlockEdit(Config{EnableHints: true}, ceStructSpec(), 100, 40)
	must.Equal(blockEditPanelTree, be.active)

	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: tea.KeyTab})
	must.Equal(blockEditPanelYAML, be.active)
	must.True(be.yamlEditor.Focused())

	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: tea.KeyTab})
	must.Equal(blockEditPanelTree, be.active)
	must.False(be.yamlEditor.Focused())

	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	must.Equal(blockEditPanelHint, be.active)
	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: tea.KeyTab})
	must.Equal(blockEditPanelTree, be.active, "tab from the hint goes back where it came from")
}

// Two legend rows: getting around, the hint panel included, help last; then
// what changes the document. Wide enough for the first row to fit whole.
func TestTheListLegendReadsInItsTwoRows(t *testing.T) {
	m := newHintModel(t, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	rows := strings.Split(ansi.Strip(updated.(model).viewContent()), "\n")
	first, second := strings.TrimSpace(rows[len(rows)-2]), strings.TrimSpace(rows[len(rows)-1])
	require.Equal(t, "[tab] change pane  [↑/↓] move  [enter/→] add  [/] filter  [ctrl+h] focus hint  [h] hide hint  [q] quit  [?] help", first)
	require.Equal(t, "[ctrl+s] save  [ctrl+r] reload  [ctrl+u] undo  [ctrl+y] redo  [ctrl+l] validate", second)
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

// ? opens the help panel over the screen; the legend under it stays the same.
func TestHelpLeavesTheLegendAsItWas(t *testing.T) {
	m := newHintModel(t, 0)
	legendOf := func(m model) string {
		rows := strings.Split(ansi.Strip(m.viewContent()), "\n")
		return strings.Join(rows[len(rows)-2:], "\n")
	}
	before := legendOf(m)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = updated.(model)
	require.Equal(t, paneAlert, m.mode, "? did not open the help panel")
	require.Equal(t, before, legendOf(m))
}

// The validation alert, like every modal, leaves the legend as it was.
func TestAnAlertLeavesTheLegendAsItWas(t *testing.T) {
	m := newHintModel(t, 0)
	legendOf := func(m model) string {
		rows := strings.Split(ansi.Strip(m.viewContent()), "\n")
		return strings.Join(rows[len(rows)-2:], "\n")
	}
	before := legendOf(m)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)
	updated, _ = updated.Update(cmd())
	m = updated.(model)
	require.Equal(t, paneAlert, m.mode, "ctrl+l did not open the validation alert")
	require.Equal(t, before, legendOf(m))
}

// → opens a block from the list, as it opens a node in the block editor.
func TestRightOpensTheSelectedBlock(t *testing.T) {
	m := newHintModel(t, 0)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	require.NotNil(t, cmd, "→ chose nothing")
	updated, _ = updated.Update(cmd())
	require.Equal(t, paneBlockEdit, updated.(model).mode)
}
