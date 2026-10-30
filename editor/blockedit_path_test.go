package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/blocklist"
	"github.com/lucasassuncao/yedit/schema"
)

// importSpec is a top-level list of strings, the shape of a list of paths.
func importSpec(content string) blockSpec {
	return blockSpec{key: "import", kind: schema.KindList, content: content}
}

// relativeDotSlash writes a pick relative to the document's folder, "./a/b".
func relativeDotSlash(abs, docPath string) (string, error) {
	rel, err := filepath.Rel(filepath.Dir(docPath), abs)
	if err != nil {
		return "", err
	}
	return "./" + filepath.ToSlash(rel), nil
}

// pathConfFolder lays out conf/{main.yaml, parts/a.yaml, parts/b.yaml}.
func pathConfFolder(t *testing.T) (docPath string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "parts"), 0o750))
	for _, name := range []string{"main.yaml", "parts/a.yaml", "parts/b.yaml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("categories: []\n"), 0o600))
	}
	return filepath.Join(dir, "main.yaml")
}

func pathConfig(docPath string) Config {
	return Config{Path: docPath, PathFields: map[string]PathField{
		"import": {Types: []string{".yaml"}, Format: relativeDotSlash},
	}}
}

func legendKeys(be blockEditState) []string {
	var out []string
	for _, a := range be.actions() {
		out = append(out, a.Entry().Help.Key)
	}
	return out
}

// runCmd runs cmd and feeds every message back into be, following batches and
// sequences, the way the program would. A message that does not come at once
// is a timer and is dropped.
func runCmd(t *testing.T, be blockEditState, cmd tea.Cmd) blockEditState {
	t.Helper()
	if cmd == nil {
		return be
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-got:
	case <-time.After(200 * time.Millisecond):
		return be
	}
	if msg == nil {
		return be
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := range v.Len() {
			be = runCmd(t, be, v.Index(i).Interface().(tea.Cmd))
		}
		return be
	}
	be, cmd = be.Update(msg)
	return runCmd(t, be, cmd)
}

func pathKey(t *testing.T, be blockEditState, k tea.KeyPressMsg) blockEditState {
	t.Helper()
	be, cmd := be.Update(k)
	return runCmd(t, be, cmd)
}

func TestPathFieldOffersThePickerOnlyWhereConfigured(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), importSpec("import:\n  - ./parts/a.yaml\n"), 100, 30)
	require.Contains(t, legendKeys(be), "ctrl+o")

	other := newBlockEdit(pathConfig(docPath), blockSpec{key: "hooks", kind: schema.KindList, content: "hooks:\n"}, 100, 30)
	require.NotContains(t, legendKeys(other), "ctrl+o")
}

// The whole flow: ctrl+o opens the picker in the document's folder, the pick
// is shaped by Format and appended, and the picker closes.
func TestPathFieldAppendsTheFormattedPick(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), importSpec("import:\n  - ./parts/a.yaml\n"), 100, 30)
	be.active = blockEditPanelTree

	be = pathKey(t, be, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Equal(t, modePicking, be.mode)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter}) // into parts/
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyDown})  // b.yaml
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Equal(t, modeEditing, be.mode, "picking closes the picker")
	require.False(t, be.sh.HasOverlay())
	require.Equal(t, "import:\n  - ./parts/a.yaml\n  - ./parts/b.yaml\n", be.yamlEditor.Value())
	require.True(t, be.dirty)

	be = be.dispatch(Undo{})
	require.Equal(t, "import:\n  - ./parts/a.yaml\n", be.yamlEditor.Value(), "a pick is one undo step")
}

// With the cursor on an entry in the YAML panel, the pick replaces it.
func TestPathFieldReplacesTheEntryUnderTheCursor(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), importSpec("import:\n  - ./parts/a.yaml\n  - ./old.yaml\n"), 100, 30)
	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue(be.yamlEditor.Value())
	be.yamlEditor = be.yamlEditor.MoveTo(2, be.yamlEditor.Column())
	require.Equal(t, 2, be.yamlEditor.Line())

	be, _ = be.Update(pathPickedMsg{abs: filepath.Join(filepath.Dir(docPath), "parts", "b.yaml")})
	require.Equal(t, "import:\n  - ./parts/a.yaml\n  - ./parts/b.yaml\n", be.yamlEditor.Value())
}

// An empty block becomes a list holding the pick.
func TestPathFieldStartsAListInAnEmptyBlock(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), importSpec("import:\n"), 100, 30)
	be, _ = be.Update(pathPickedMsg{abs: filepath.Join(filepath.Dir(docPath), "parts", "a.yaml")})
	require.Equal(t, "import:\n  - ./parts/a.yaml\n", be.yamlEditor.Value())
}

// The relative path must hold where the file is written, so Format sees the
// save target when --output points elsewhere.
func TestPathFieldFormatsAgainstTheSaveTarget(t *testing.T) {
	docPath := pathConfFolder(t)
	cfg := pathConfig(docPath)
	cfg.SavePath = filepath.Join(filepath.Dir(docPath), "parts", "copy.yaml")
	be := newBlockEdit(cfg, importSpec("import:\n"), 100, 30)
	be, _ = be.Update(pathPickedMsg{abs: filepath.Join(filepath.Dir(docPath), "parts", "a.yaml")})
	require.Equal(t, "import:\n  - ./a.yaml\n", be.yamlEditor.Value())
}

// In the navigator, a pick from the tree adds an entry and shows it.
func TestPathFieldPickAddsAnEntryInTheNavigator(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), scalarListSpec("import:\n  - ./parts/a.yaml\n"), 100, 30)

	be, _ = be.Update(pathPickedMsg{abs: filepath.Join(filepath.Dir(docPath), "parts", "b.yaml")})

	require.Equal(t, []string{"./parts/a.yaml", "./parts/b.yaml"}, seqItemLabels(be))
	require.Equal(t, 1, be.coll.current)
	require.Equal(t, "import:\n  - ./parts/b.yaml\n", be.yamlEditor.Value())

	be = be.dispatch(Undo{})
	require.Equal(t, []string{"./parts/a.yaml"}, seqItemLabels(be), "a pick is one undo step")
}

// Editing an entry in the navigator, the pick replaces it.
func TestPathFieldPickReplacesTheEntryBeingEdited(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), scalarListSpec("import:\n  - ./old.yaml\n"), 100, 30)
	be.active = blockEditPanelYAML

	be, _ = be.Update(pathPickedMsg{abs: filepath.Join(filepath.Dir(docPath), "parts", "b.yaml")})

	require.Equal(t, []string{"./parts/b.yaml"}, seqItemLabels(be))
	require.Equal(t, "import:\n  - ./parts/b.yaml\n", be.yamlEditor.Value())
}

// openingConfig is pathConfig whose import entries open; Open records what it got.
func openingConfig(docPath string, got *[2]string, err error) Config {
	cfg := pathConfig(docPath)
	pf := cfg.PathFields["import"]
	pf.Open = func(value, doc string) (Config, error) {
		got[0], got[1] = value, doc
		return Config{Path: value}, err
	}
	cfg.PathFields["import"] = pf
	return cfg
}

// Enter on an entry asks to open the file it names, with the Config Open gives.
func TestPathFieldEnterOpensTheEntry(t *testing.T) {
	docPath := pathConfFolder(t)
	var got [2]string
	be := newBlockEdit(openingConfig(docPath, &got, nil), scalarListSpec("import:\n  - ./parts/a.yaml\n  - ./parts/b.yaml\n"), 100, 30)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyDown})
	var enter string
	for _, a := range be.actions() {
		if a.Entry().Help.Key == "enter" {
			enter = a.Entry().Help.Desc
		}
	}
	require.Equal(t, "open file", enter)

	_, cmd := be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.NotNil(t, cmd)
	require.Equal(t, openFileMsg{cfg: Config{Path: "./parts/b.yaml"}}, cmd())
	require.Equal(t, [2]string{"./parts/b.yaml", be.docPath()}, got)
}

// An Open error is shown in the block editor, and nothing opens.
func TestPathFieldOpenErrorIsShown(t *testing.T) {
	docPath := pathConfFolder(t)
	var got [2]string
	be := newBlockEdit(openingConfig(docPath, &got, os.ErrNotExist), scalarListSpec("import:\n  - ./gone.yaml\n"), 100, 30)

	be, cmd := be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Nil(t, cmd)
	require.Contains(t, be.editorErr.message, "./gone.yaml")
}

// A path list without Open keeps Enter as it was: nothing opens.
func TestPathFieldEnterWithoutOpenDoesNothing(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), scalarListSpec("import:\n  - ./parts/a.yaml\n"), 100, 30)

	_, cmd := be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Nil(t, cmd)
}

// A Format that cannot shape the pick leaves the block alone and says why.
func TestPathFieldReportsAFormatError(t *testing.T) {
	docPath := pathConfFolder(t)
	cfg := pathConfig(docPath)
	cfg.PathFields["import"] = PathField{Format: func(string, string) (string, error) {
		return "", os.ErrInvalid
	}}
	be := newBlockEdit(cfg, importSpec("import:\n"), 100, 30)
	be, _ = be.Update(pathPickedMsg{abs: docPath})
	require.Equal(t, "import:\n", be.yamlEditor.Value())
	require.Contains(t, be.editorErr.message, "invalid argument")
}

// A scalar path field takes the pick as its value; nil Format writes it whole.
func TestPathFieldSetsAScalar(t *testing.T) {
	docPath := pathConfFolder(t)
	cfg := Config{Path: docPath, PathFields: map[string]PathField{"logfile": {}}}
	be := newBlockEdit(cfg, blockSpec{key: "logfile", kind: schema.KindPrimitive, content: "logfile: old.log\n"}, 100, 30)
	be, _ = be.Update(pathPickedMsg{abs: docPath})
	require.Equal(t, "logfile: "+docPath+"\n", be.yamlEditor.Value())
}

// esc closes the picker without touching the block.
func TestPathFieldPickerCancels(t *testing.T) {
	docPath := pathConfFolder(t)
	be := newBlockEdit(pathConfig(docPath), importSpec("import:\n"), 100, 30)
	be.active = blockEditPanelTree
	be = pathKey(t, be, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, modeEditing, be.mode)
	require.False(t, be.sh.HasOverlay())
	require.Equal(t, "import:\n", be.yamlEditor.Value())
}

// importDoc is a document whose top-level import is a list of paths.
type importDoc struct {
	Import []string `yaml:"import"`
}

// runModel runs cmd and feeds what it answers back into m, as the program would.
func runModel(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		return m
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-got:
	case <-time.After(200 * time.Millisecond):
		return m
	}
	if msg == nil {
		return m
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
		for i := range v.Len() {
			m = runModel(t, m, v.Index(i).Interface().(tea.Cmd))
		}
		return m
	}
	u, cmd := m.Update(msg)
	return runModel(t, u.(model), cmd)
}

func sendModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	u, cmd := m.Update(msg)
	return runModel(t, u.(model), cmd)
}

// Through the whole program: the root routes the picker's keys, its directory
// listings and its close to the block editor, and ctrl+s writes the pick.
func TestPathFieldPickIsSavedThroughTheProgram(t *testing.T) {
	docPath := pathConfFolder(t)
	require.NoError(t, os.WriteFile(docPath, []byte("import:\n  - ./parts/a.yaml\n"), 0o600))
	cfg := pathConfig(docPath)
	cfg.Schema = &importDoc{}
	cfg.NoSaveConfirm = true
	m, err := newModel(cfg)
	require.NoError(t, err)
	m = sendModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = sendModel(t, m, blocklist.OpenItemMsg{Item: blocklist.Item{Key: "import", Existing: true}})
	require.Equal(t, paneBlockEdit, m.mode)

	for _, k := range []tea.KeyPressMsg{
		{Code: 'o', Mod: tea.ModCtrl},
		{Code: tea.KeyEnter}, // into parts/
		{Code: tea.KeyDown},  // b.yaml
		{Code: tea.KeyEnter},
		{Code: 's', Mod: tea.ModCtrl},
	} {
		m = sendModel(t, m, k)
	}
	require.Equal(t, paneList, m.mode, "ctrl+s commits the block back to the list")
	m = sendModel(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	saved, err := os.ReadFile(docPath)
	require.NoError(t, err)
	require.Equal(t, "import:\n  - ./parts/a.yaml\n  - ./parts/b.yaml\n", string(saved))
}
