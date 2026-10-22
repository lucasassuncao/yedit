package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/blocklist"
)

// importConf is a document whose import entries open in tabs of their own.
type importConf struct {
	Import []string    `yaml:"import"`
	Server serverProbe `yaml:"server"`
}

// filesModel opens main.yaml, which imports parts/a.yaml; every import entry
// opens with the same kind of Config.
func filesModel(t *testing.T) (tabbedModel, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "parts"), 0o750))
	main := filepath.Join(dir, "main.yaml")
	require.NoError(t, os.WriteFile(main, []byte("import:\n  - ./parts/a.yaml\nserver:\n  host: main\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parts", "a.yaml"), []byte("server:\n  host: a\n"), 0o600))
	var open func(value, docPath string) (Config, error)
	conf := func(path string) Config {
		return Config{Path: path, Schema: &importConf{}, PathFields: map[string]PathField{"import": {Open: open}}}
	}
	open = func(value, docPath string) (Config, error) {
		abs := filepath.Join(filepath.Dir(docPath), filepath.FromSlash(value))
		if _, err := os.Stat(abs); err != nil {
			return Config{}, err
		}
		return conf(abs), nil
	}
	root, err := newRoot(conf(main))
	require.NoError(t, err)
	m, _ := sendTabbed(root.(tabbedModel), tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, dir
}

func press(t *testing.T, m tabbedModel, msgs ...tea.Msg) tabbedModel {
	t.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = sendTabbed(m, msg)
		m = runTabbed(t, m, cmd)
	}
	return m
}

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlS    = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
)

// openA opens the import block and presses enter on its one entry.
func openA(t *testing.T, m tabbedModel) tabbedModel {
	t.Helper()
	return press(t, m, blocklist.OpenItemMsg{Item: blocklist.Item{Key: "import", Existing: true}}, enterKey)
}

func TestEnterOnAnImportOpensItInATab(t *testing.T) {
	m, _ := filesModel(t)
	assert.NotContains(t, screen(m), "ctrl+t", "one screen needs no strip")
	assert.Equal(t, 40, m.editor.height)

	m = openA(t, m)

	require.Len(t, m.files, 1)
	assert.Equal(t, 1, m.active, "the file comes in front")
	out := screen(m)
	rows := strings.Split(out, "\n")
	assert.Contains(t, rows[0], "a.yaml")
	assert.Regexp(t, `^ main\.yaml   a\.yaml +$`, rows[1], "the strip comes with the second screen")
	assert.Contains(t, out, "[ctrl+t] change tab")
	assert.Contains(t, out, "host: a")
	assert.Equal(t, 39, m.editor.height, "every screen gives the strip a row")
	assert.Equal(t, 39, m.files[0].m.height)
}

// A file tab saves its own file, through its own review; the main document
// and its editor are left alone.
func TestAFileTabSavesItsOwnFile(t *testing.T) {
	m, dir := filesModel(t)
	m = openA(t, m)
	doc, err := m.files[0].m.doc.ReplaceRaw([]byte("server:\n  host: b\n"))
	require.NoError(t, err)
	m.files[0].m.doc = doc

	m = press(t, m, ctrlS)
	assert.Contains(t, screen(m), "Save changes?")
	assert.Contains(t, screen(m), "a.yaml")
	m = press(t, m, enterKey)

	saved, err := os.ReadFile(filepath.Join(dir, "parts", "a.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "server:\n  host: b\n", string(saved))
	main, err := os.ReadFile(filepath.Join(dir, "main.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(main), "host: main")
	assert.False(t, m.files[0].m.doc.Dirty())
	assert.Equal(t, paneBlockEdit, m.editor.mode, "the main editor never saw the file tab's messages")
}

func TestEscClosesACleanFileTab(t *testing.T) {
	m, _ := filesModel(t)
	m = openA(t, m)

	m = press(t, m, escKey)

	assert.Empty(t, m.files)
	assert.Equal(t, 0, m.active)
	assert.Equal(t, 40, m.editor.height, "the strip leaves with the last other screen")
	assert.NotContains(t, screen(m), "ctrl+t")
}

func TestEscAsksBeforeClosingAnEditedFileTab(t *testing.T) {
	m, _ := filesModel(t)
	m = openA(t, m)
	doc, err := m.files[0].m.doc.ReplaceRaw([]byte("server:\n  host: b\n"))
	require.NoError(t, err)
	m.files[0].m.doc = doc

	m = press(t, m, escKey)
	require.Len(t, m.files, 1, "nothing closes before the answer")
	assert.Contains(t, screen(m), "Close without saving?")

	m = press(t, m, enterKey)
	assert.Empty(t, m.files)
}

// editA leaves an unsaved change in the file tab.
func editA(t *testing.T, m tabbedModel) tabbedModel {
	t.Helper()
	doc, err := m.files[0].m.doc.ReplaceRaw([]byte("server:\n  host: b\n"))
	require.NoError(t, err)
	m.files[0].m.doc = doc
	return m
}

// A file tab closes with esc and never quits the program: only the main
// screen offers q.
func TestAFileTabClosesAndTheMainScreenQuits(t *testing.T) {
	m, _ := filesModel(t)
	m = openA(t, m)

	out := screen(m)
	assert.Contains(t, out, "\n[esc] close tab  [ctrl+t] change tab  [tab] change pane", "closing the tab leads the legend")
	assert.NotContains(t, out, "[q] quit")

	m = press(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	assert.Len(t, m.files, 1, "q does nothing in a file tab")
	assert.Equal(t, 1, m.active)
	assert.Equal(t, paneList, m.files[0].m.mode, "no question was asked")
	assert.NotEqual(t, paneAlert, m.editor.mode)

	m = press(t, m, ctrlT)
	require.Equal(t, 0, m.active)
	m = press(t, m, escKey) // leave the import block editor openA left open
	out = screen(m)
	assert.Contains(t, out, "[q] quit")
	assert.Contains(t, out, "[ctrl+t] change tab")
	assert.NotContains(t, out, "close tab")
}

// ctrl+c quits from a file tab too, with one question about every file
// rather than the tab's own first.
func TestCtrlCInAFileTabAsksOnceAboutEveryFile(t *testing.T) {
	m, _ := filesModel(t)
	m = editA(t, openA(t, m))

	m = press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	assert.Equal(t, 0, m.active, "the main editor asks")
	assert.Equal(t, paneAlert, m.editor.mode)
	assert.NotEqual(t, paneAlert, m.files[0].m.mode, "the file tab asked nothing of its own")
	assert.Contains(t, screen(m), "a.yaml has unsaved changes")
}

// ctrl+t only changes the screen in front: a file tab keeps its edits until
// it is closed.
func TestCtrlTKeepsAFileTabAndItsEdits(t *testing.T) {
	m, _ := filesModel(t)
	m = editA(t, openA(t, m))

	m = press(t, m, ctrlT, ctrlT)

	require.Len(t, m.files, 1)
	assert.Equal(t, 1, m.active)
	assert.True(t, m.files[0].m.doc.Dirty())
	assert.Contains(t, string(m.files[0].m.doc.Raw()), "host: b")
}

// esc on a file tab's preview goes back to its block list; only there does it
// close the tab.
func TestEscOnAFileTabsPreviewGoesBack(t *testing.T) {
	m, _ := filesModel(t)
	m = openA(t, m)
	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, panePreview, m.files[0].m.mode)
	assert.Contains(t, screen(m), "[esc] back")

	m = press(t, m, escKey)

	require.Len(t, m.files, 1)
	assert.Equal(t, paneList, m.files[0].m.mode)
}

func TestOpeningAnOpenFileBringsItToFront(t *testing.T) {
	m, _ := filesModel(t)
	m = openA(t, m)
	m = press(t, m, ctrlT)
	require.Equal(t, 0, m.active)

	m = press(t, m, enterKey)

	assert.Len(t, m.files, 1)
	assert.Equal(t, 1, m.active)
}
