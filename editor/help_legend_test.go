package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

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
	be.yamlEditor.Blur()
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
	be.yamlEditor.Blur()

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
	be.yamlEditor.Blur()

	rows := strings.Split(legendRow(be.View(nil)), "\n")
	require.Equal(t, "[tab] change pane  [↑/↓] scroll  [h] hide hint  [esc] back  [?] help", strings.TrimSpace(rows[1]))
	require.NotContains(t, rows[0], "[ctrl+s]", "no second row of document keys")
}

// esc on the hint returns to the pane it came from: it must not leave the
// block, nor ask to discard its changes.
func TestEscOnTheHintReturnsToThePreviousPane(t *testing.T) {
	be := newBlockEdit(Config{EnableHints: true}, ceStructSpec(), 100, 40)
	be.showHint = true
	be.dirty = true
	be.prevActive, be.active = blockEditPanelYAML, blockEditPanelHint

	be, cmd := pressBEAction(t, be, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, blockEditPanelYAML, be.active)
	require.Equal(t, modeEditing, be.mode, "no discard prompt")
	require.False(t, be.sh.HasOverlay())
	if cmd != nil {
		_, left := cmd().(blockEditDiscardedMsg)
		require.False(t, left, "esc must not leave the block")
	}
}

// ctrl+h is not offered on the hint, but it still gives the keys back: it is
// how the reader got there, and the same key undoes it.
func TestCtrlHOnTheHintStillReturns(t *testing.T) {
	be := newBlockEdit(Config{EnableHints: true}, ceStructSpec(), 100, 40)
	be.showHint = true
	be.prevActive, be.active = blockEditPanelTree, blockEditPanelHint

	be, _ = be.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	require.Equal(t, blockEditPanelTree, be.active)
}

// The main screen's hint reads the same way: one row, no document keys.
func TestTheMainScreenHintLegendIsOneRow(t *testing.T) {
	m := newHintModel(t, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(model).toggleHintFocus()
	require.Equal(t, paneHint, m.mode)

	rows := strings.Split(legendRow(m.viewContent()), "\n")
	require.Equal(t, "[tab] change pane  [↑/↓] scroll  [h] hide hint  [esc] back  [q] quit  [?] help", strings.TrimSpace(rows[1]))
	require.NotContains(t, rows[0], "[ctrl+s]", "no second row of document keys")
}

// On the YAML panel "?" is text: it must type, and the legend must not offer it.
func TestYAMLPanelKeepsQuestionMarkAsText(t *testing.T) {
	be := newServerBlock(t)
	be.active = blockEditPanelYAML
	be.yamlEditor.Focus()
	require.NotContains(t, legendRow(be.View(nil)), "[?] help")

	be, _ = be.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.False(t, be.sh.HasOverlay())
	require.Contains(t, be.yamlEditor.Value(), "?")
}
