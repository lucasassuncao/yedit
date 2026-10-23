package editor

import (
	"os"
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

// legendRow is the bottom two rows, where the legend lives.
func legendRow(screen string) string {
	rows := strings.Split(ansi.Strip(screen), "\n")
	return strings.Join(rows[len(rows)-2:], "\n")
}

func TestListLegendPinsHelp(t *testing.T) {
	m := newHintModel(t, 0)
	require.Contains(t, legendRow(m.viewContent()), "[?] help")
}

// While filtering "?" is typed into the filter, so the legend must not offer it.
func TestFilteringLegendHidesHelp(t *testing.T) {
	m := newHintModel(t, 0)
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(model)
	require.True(t, m.list.IsFiltering())
	require.NotContains(t, legendRow(m.viewContent()), "[?] help")
}

func newServerBlock(t *testing.T) blockEditState {
	t.Helper()
	path := filepath.Join(t.TempDir(), "help.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	require.NoError(t, err)
	return newBlockEdit(m.cfg, blockSpec{key: "server"}, 100, 30)
}

func TestBlockEditorHelpIsShownAndOpens(t *testing.T) {
	be := newServerBlock(t)
	be.active = blockEditPanelTree
	be.yamlEditor = be.yamlEditor.Blur()
	require.Contains(t, legendRow(be.View(nil)), "[?] help")

	be, _ = be.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.True(t, be.sh.HasOverlay(), "? opens the help")
	require.Equal(t, modeConfirming, be.mode)
}

// The tree's two rows: getting around, the hint panel included, help last;
// then what changes the block.
func TestTheBlockEditorLegendReadsInItsTwoRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "help.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}, EnableHints: true})
	require.NoError(t, err)
	be := newBlockEdit(m.cfg, blockSpec{key: "server"}, 140, 30)
	be.active = blockEditPanelTree
	be.yamlEditor = be.yamlEditor.Blur()

	rows := strings.Split(legendRow(be.View(nil)), "\n")
	require.Equal(t, "[tab] change pane  [↑/↓] move  [→/←] expand  [ctrl+h] focus hint  [h] hide hint  [esc] back  [?] help",
		strings.TrimSpace(rows[0]))
	require.Equal(t, "[enter] add  [ctrl+s] save changes  [ctrl+d] remove  [ctrl+u] undo  [ctrl+y] redo  [ctrl+l] validate",
		strings.TrimSpace(rows[1]))
}

// The focused hint is for reading: one row, no document keys, and no
// ctrl+h, which is how the reader got there.
func TestTheHintPanelLegendIsOneRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "help.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}, EnableHints: true})
	require.NoError(t, err)
	be := newBlockEdit(m.cfg, blockSpec{key: "server"}, 140, 30)
	be.showHint = true
	be.prevActive, be.active = blockEditPanelTree, blockEditPanelHint
	be.yamlEditor = be.yamlEditor.Blur()

	rows := strings.Split(legendRow(be.View(nil)), "\n")
	require.Equal(t, "[tab] change pane  [↑/↓] scroll  [h] hide hint  [esc] back  [?] help", strings.TrimSpace(rows[0]))
	require.Empty(t, strings.TrimSpace(rows[1]), "no second row of document keys")
}

// The main screen's hint reads the same way: one row, no document keys.
func TestTheMainScreenHintLegendIsOneRow(t *testing.T) {
	m := newHintModel(t, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(model).toggleHintFocus()
	require.Equal(t, paneHint, m.mode)

	rows := strings.Split(legendRow(m.viewContent()), "\n")
	require.Equal(t, "[tab] change pane  [↑/↓] scroll  [h] hide hint  [esc] back  [q] quit  [?] help", strings.TrimSpace(rows[0]))
	require.Empty(t, strings.TrimSpace(rows[1]), "no second row of document keys")
}

// On the YAML panel "?" is text: it must type, and the legend must not offer it.
func TestYAMLPanelKeepsQuestionMarkAsText(t *testing.T) {
	be := newServerBlock(t)
	be.active = blockEditPanelYAML
	be.yamlEditor, _ = be.yamlEditor.Focus()
	require.NotContains(t, legendRow(be.View(nil)), "[?] help")

	be, _ = be.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.False(t, be.sh.HasOverlay())
	require.Contains(t, be.yamlEditor.Value(), "?")
}

func TestActionsAreInTheLegendAndBoundToTheirKey(t *testing.T) {
	m, _ := actionModel(t, []Action{{Key: "ctrl+e", Help: "save & convert", SaveFirst: true, Run: (&recorder{}).run}})

	require.Contains(t, ansi.Strip(m.View().Content), "save & convert")
	_, msgs := step(t, m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	require.Equal(t, []tea.Msg{actionRequestedMsg{key: "ctrl+e"}}, msgs)
}
