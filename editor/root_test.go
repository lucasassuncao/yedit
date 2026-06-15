package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/internal/alert"
)

// sizeProbeConfig is a minimal schema with one struct block so the test can
// open a block-edit screen.
type sizeProbeConfig struct {
	Server serverProbe `yaml:"server"`
}

type serverProbe struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// TestWindowSizeReachesBlockEdit guards against a regression where the root
// Update consumed tea.WindowSizeMsg and returned before forwarding it to the
// open block-edit sub-model, leaving its panels at stale dimensions.
func TestWindowSizeReachesBlockEdit(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "probe.yaml"),
		Schema: &sizeProbeConfig{},
	})
	must.NoError(err, "newModel")

	// Establish an initial terminal size.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open the "server" block to enter block-edit mode.
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server"}})
	m = updated.(model)
	must.Equal(paneBlockEdit, m.mode, "expected paneBlockEdit")
	must.NotNil(m.topBE(), "blockEdit must be non-nil")

	// Resize the terminal while the block editor is open.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = updated.(model)

	top := m.topBE()
	must.NotNil(top, "blockEdit went nil after resize")
	is.Equal(120, top.width, "block edit not resized: width")
	is.Equal(50, top.height, "block edit not resized: height")
}

// TestPreviewIsReadOnly verifies that typing in the preview pane never mutates
// the document - the right panel is a read-only, syntax-highlighted view.
func TestPreviewIsReadOnly(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "ro.yaml"),
		Schema: &sizeProbeConfig{},
	})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Seed some content, then enter the preview pane via Tab.
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server"}})
	m = updated.(model)
	updated, _ = m.Update(blockEditCommittedMsg{Snippet: `server:
  host: localhost
`})
	m = updated.(model)
	updated, _ = m.Update(blockEditDiscardedMsg{})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	must.Equal(panePreview, m.mode, "expected panePreview after Tab")

	before := string(m.doc.Raw())
	dirtyBefore := m.doc.Dirty()

	// Type characters - a read-only preview must ignore them.
	for _, r := range "xyz: hello" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}

	is.Equal(before, string(m.doc.Raw()), "preview edited the document")
	is.Equal(dirtyBefore, m.doc.Dirty(), "preview changed dirty state")
}

// TestCtrlU_blockEditorNoSnapDoesNotTouchDocument verifies that pressing ctrl+u
// inside a block editor when the undo stack is empty is a no-op: the document
// and the editor mode must be unchanged. Without this guard the fallback
// m.doc.Undo() would revert the document while the editor is still open,
// leaving it showing stale content.
func TestCtrlU_blockEditorNoSnapDoesNotTouchDocument(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	must.NoError(os.WriteFile(path, []byte(`server:
  host: localhost
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open the "server" block editor.
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server", Existing: true}})
	m = updated.(model)
	must.Equal(paneBlockEdit, m.mode, "expected paneBlockEdit")
	must.Empty(m.topBE().undoStack, "undo stack should be empty on a freshly opened editor")

	rawBefore := string(m.doc.Raw())
	canUndoBefore := m.doc.CanUndo()

	// ctrl+u with an empty undo stack must be a no-op.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = updated.(model)

	is.Equal(paneBlockEdit, m.mode, "ctrl+u changed pane")
	is.Equal(rawBefore, string(m.doc.Raw()), "ctrl+u modified the document")
	is.Equal(canUndoBefore, m.doc.CanUndo(), "ctrl+u consumed a document history entry")
}

// TestBuildListItemsAvailableKeepsCanonicalOrder verifies AVAILABLE keys follow
// the schema's declaration order (not alphabetical), matching Insert placement.
func TestBuildListItemsAvailableKeepsCanonicalOrder(t *testing.T) {
	is := assert.New(t)
	known := []string{"name", "image", "build", "appPort"} // canonical, not alphabetical
	var got []string
	for _, it := range buildListItems(known, nil, nil) {
		if !it.Separator {
			got = append(got, it.Key)
		}
	}
	want := []string{"name", "image", "build", "appPort"}
	is.Equal(want, got, "available keys should preserve canonical order, not alphabetical")
}

// TestListMoveCursorClampsAtBounds verifies the main list clamps at top/bottom
// instead of wrapping around, matching the tree panel.
func TestListMoveCursorClampsAtBounds(t *testing.T) {
	is := assert.New(t)
	lm := newListModel([]string{"a", "b", "c"}, nil, nil, 10)
	first := lm.cursor
	lm = lm.moveCursor(-1) // already at the top - must not wrap to the bottom
	is.Equal(first, lm.cursor, "moveCursor(-1) at top should clamp, not wrap")
	lm = lm.jumpToLast()
	last := lm.cursor
	lm = lm.moveCursor(1) // at the bottom - must not wrap to the top
	is.Equal(last, lm.cursor, "moveCursor(+1) at bottom should clamp, not wrap")
}

// followCfg is a flat schema used to exercise preview-follows-selection.
type followCfg struct {
	A string `yaml:"a"`
	B string `yaml:"b"`
	C string `yaml:"c"`
	D string `yaml:"d"`
	E string `yaml:"e"`
	F string `yaml:"f"`
}

// TestPreviewFollowsSelectedBlock verifies that navigating the ADDED list
// scrolls the read-only preview to the selected block's line.
func TestPreviewFollowsSelectedBlock(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "f.yaml")
	must.NoError(os.WriteFile(path, []byte(`a: 1
b: 2
c: 3
d: 4
e: 5
f: 6
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &followCfg{}})
	must.NoError(err, "newModel")
	// Short viewport so the 6-line document overflows and can actually scroll.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = updated.(model)
	must.Equal(0, m.preview.YOffset, "initial YOffset should be 0")
	// Navigate down to "c" (third block, line 3).
	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	it := m.list.SelectedItem()
	must.NotNil(it, "selected item should not be nil")
	must.Equal("c", it.Key, "expected selected item c")
	is.Equal(2, m.preview.YOffset, "YOffset following \"c\" should be 2 (block at line 3)")
}

// checkScreenInvariant asserts the two screen invariants that the enter* helpers
// are meant to guarantee:
//
//	m.alert != nil        ⟺  m.mode == paneAlert
//	len(m.blockEdits) > 0  ⟺  m.mode == paneBlockEdit
func checkScreenInvariant(t *testing.T, m model, where string) {
	t.Helper()
	if m.alertVisible != (m.mode == paneAlert) {
		t.Errorf("%s: alert/mode invariant broken: alertVisible=%v mode=%d", where, m.alertVisible, m.mode)
	}
	if (len(m.blockEdits) > 0) != (m.mode == paneBlockEdit) {
		t.Errorf("%s: blockEdits/mode invariant broken: len=%d mode=%d", where, len(m.blockEdits), m.mode)
	}
}

// TestScreenInvariantAcrossTransitions drives the model through every reachable
// screen transition and asserts the mode/data invariants hold at each step. This
// guards the centralized enter* transitions against a future raw `m.mode = …`
// that forgets to clear a sibling field.
func TestScreenInvariantAcrossTransitions(t *testing.T) {
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "inv.yaml")
	must.NoError(os.WriteFile(path, []byte(`server:
  host: localhost
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)
	checkScreenInvariant(t, m, "initial list")

	// list → block edit
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server", Existing: true}})
	m = updated.(model)
	checkScreenInvariant(t, m, "after openItem")

	// block edit → list (discard)
	updated, _ = m.Update(blockEditDiscardedMsg{})
	m = updated.(model)
	checkScreenInvariant(t, m, "after discard")

	// list → preview → list
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	checkScreenInvariant(t, m, "after tab to preview")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	checkScreenInvariant(t, m, "after tab back to list")

	// list → alert (save confirm) → list (dismiss)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	checkScreenInvariant(t, m, "after ctrl+s save confirm")
	if m.mode != paneAlert {
		t.Fatalf("expected paneAlert after ctrl+s from list, got %d", m.mode)
	}
	updated, _ = m.Update(alert.DismissedMsg{})
	m = updated.(model)
	checkScreenInvariant(t, m, "after alert dismiss")
}

// TestListFilterByTyping verifies the "/" filter narrows the list as the user types.
func TestListFilterByTyping(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	lm := newListModel([]string{"alpha", "beta", "gamma"}, nil, nil, 10)
	must.False(lm.IsFiltering(), "should not start in filtering mode")
	lm, _ = lm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	must.True(lm.IsFiltering(), `"/" should enter filtering mode`)
	for _, r := range "be" {
		lm, _ = lm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	got := lm.filteredItems()
	if is.Len(got, 1, `filter "be" should match exactly one item`) {
		is.Equal("beta", got[0].Key, `filter "be" should match beta`)
	}
}

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
	view := m.View()
	is.Contains(view, "Hint/Example", "view should show the Hint/Example panel when EnableHints is true")
	is.Contains(view, "object", "hint should show the selected field's type (server → object)")

	// "h" hides the panel.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(model)
	must.False(m.showHint, "pressing h should hide the hint panel")
	is.NotContains(m.View(), "Hint/Example", "hint panel should be hidden after pressing h")

	// "h" again shows it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(model)
	must.True(m.showHint, "pressing h again should re-enable the hint panel")
	is.Contains(m.View(), "Hint/Example", "hint panel should be visible after toggling back on")
}

// TestReloadFromDisk covers ctrl+r in the main list: a clean document reloads
// immediately, a dirty one prompts first and reloads after confirmation.
func TestReloadFromDisk(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "reload.yaml")
	must.NoError(os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Clean document: ctrl+r dispatches an async reload cmd; execute it.
	must.NoError(os.WriteFile(path, []byte("server:\n  host: b\n"), 0o600))
	var reloadCmd tea.Cmd
	updated, reloadCmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(model)
	must.Equal(paneList, m.mode, "clean reload should not prompt")
	if reloadCmd != nil {
		updated, _ = m.Update(reloadCmd())
		m = updated.(model)
	}
	is.Contains(string(m.doc.Raw()), "host: b", "external change not loaded")

	// Dirty document: ctrl+r prompts; confirming discards the local edit.
	m.doc, err = m.doc.Insert("extra: 1\n")
	must.NoError(err, "Insert")
	must.NoError(os.WriteFile(path, []byte("server:\n  host: c\n"), 0o600))
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(model)
	must.Equal(paneAlert, m.mode, "dirty reload should prompt")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(model)
	must.NotNil(cmd, "confirming the alert should produce a command")
	// cmd() fires confirmedReloadMsg → execReload returns cmdReload
	updated, reloadCmd = m.Update(cmd())
	m = updated.(model)
	if reloadCmd != nil {
		updated, _ = m.Update(reloadCmd())
		m = updated.(model)
	}
	must.Equal(paneList, m.mode, "expected list after confirmed reload")
	is.Contains(string(m.doc.Raw()), "host: c", "reload did not replace local state")
	is.NotContains(string(m.doc.Raw()), "extra", "reload did not discard local edit")
	is.False(m.doc.Dirty(), "reloaded document should not be dirty")
}

// TestFilterAcceptsJK guards against j/k being swallowed as navigation while
// the filter input is active - filters like "unknown" (contains k) or
// "worker" (contains k) must be typeable; only the arrow keys navigate.
func TestFilterAcceptsJK(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "filter.yaml")
	seed := "server:\n  host: a\nunknown-key: flagged\n"
	must.NoError(os.WriteFile(path, []byte(seed), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(model)
	for _, r := range "unknown" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	must.Equal("unknown", m.list.filter, "filter text after typing")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	sel := m.list.SelectedItem()
	must.NotNil(sel, "selected item should not be nil after filter+enter")
	is.Equal("unknown-key", sel.Key, "filter+enter should select unknown-key")
}
