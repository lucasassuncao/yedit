package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "probe.yaml"),
		Schema: &sizeProbeConfig{},
	})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}

	// Establish an initial terminal size.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open the "server" block to enter block-edit mode.
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server"}})
	m = updated.(model)
	if m.mode != paneBlockEdit || m.topBE() == nil {
		t.Fatalf("expected paneBlockEdit with non-nil blockEdit, got mode=%d blockEdit=%v", m.mode, m.topBE())
	}

	// Resize the terminal while the block editor is open.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = updated.(model)

	top := m.topBE()
	if top == nil {
		t.Fatal("blockEdit went nil after resize")
	}
	if top.width != 120 || top.height != 50 {
		t.Errorf("block edit not resized: got %dx%d, want 120x50",
			top.width, top.height)
	}
}

// TestPreviewIsReadOnly verifies that typing in the preview pane never mutates
// the document - the right panel is a read-only, syntax-highlighted view.
func TestPreviewIsReadOnly(t *testing.T) {
	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "ro.yaml"),
		Schema: &sizeProbeConfig{},
	})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
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
	if m.mode != panePreview {
		t.Fatalf("expected panePreview after Tab, got %d", m.mode)
	}

	before := string(m.doc.Raw())
	dirtyBefore := m.doc.Dirty()

	// Type characters - a read-only preview must ignore them.
	for _, r := range "xyz: hello" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}

	if got := string(m.doc.Raw()); got != before {
		t.Errorf("preview edited the document: raw changed from %q to %q", before, got)
	}
	if m.doc.Dirty() != dirtyBefore {
		t.Errorf("preview changed dirty state: was %v now %v", dirtyBefore, m.doc.Dirty())
	}
}

// TestCtrlU_blockEditorNoSnapDoesNotTouchDocument verifies that pressing ctrl+u
// inside a block editor when the undo stack is empty is a no-op: the document
// and the editor mode must be unchanged. Without this guard the fallback
// m.doc.Undo() would revert the document while the editor is still open,
// leaving it showing stale content.
func TestCtrlU_blockEditorNoSnapDoesNotTouchDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(path, []byte(`server:
  host: localhost
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open the "server" block editor.
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "server", Existing: true}})
	m = updated.(model)
	if m.mode != paneBlockEdit {
		t.Fatalf("expected paneBlockEdit, got %d", m.mode)
	}
	if len(m.topBE().undoStack) != 0 {
		t.Fatal("undo stack should be empty on a freshly opened editor")
	}

	rawBefore := string(m.doc.Raw())
	canUndoBefore := m.doc.CanUndo()

	// ctrl+u with an empty undo stack must be a no-op.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = updated.(model)

	if m.mode != paneBlockEdit {
		t.Errorf("ctrl+u changed pane: got %d, want %d (paneBlockEdit)", m.mode, paneBlockEdit)
	}
	if got := string(m.doc.Raw()); got != rawBefore {
		t.Errorf("ctrl+u modified the document:\n was: %q\n now: %q", rawBefore, got)
	}
	if m.doc.CanUndo() != canUndoBefore {
		t.Errorf("ctrl+u consumed a document history entry (CanUndo: %v → %v)", canUndoBefore, m.doc.CanUndo())
	}
}

// TestBuildListItemsAvailableKeepsCanonicalOrder verifies AVAILABLE keys follow
// the schema's declaration order (not alphabetical), matching Insert placement.
func TestBuildListItemsAvailableKeepsCanonicalOrder(t *testing.T) {
	known := []string{"name", "image", "build", "appPort"} // canonical, not alphabetical
	var got []string
	for _, it := range buildListItems(known, nil, nil) {
		if !it.Separator {
			got = append(got, it.Key)
		}
	}
	want := []string{"name", "image", "build", "appPort"}
	if len(got) != len(want) {
		t.Fatalf("available = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("available[%d] = %q, want %q (canonical order, not alphabetical)", i, got[i], want[i])
		}
	}
}

// TestListMoveCursorClampsAtBounds verifies the main list clamps at top/bottom
// instead of wrapping around, matching the tree panel.
func TestListMoveCursorClampsAtBounds(t *testing.T) {
	lm := newListModel([]string{"a", "b", "c"}, nil, nil, 10)
	first := lm.cursor
	lm.moveCursor(-1) // already at the top - must not wrap to the bottom
	if lm.cursor != first {
		t.Errorf("moveCursor(-1) at top moved cursor to %d, want %d (clamp)", lm.cursor, first)
	}
	lm.jumpToLast()
	last := lm.cursor
	lm.moveCursor(1) // at the bottom - must not wrap to the top
	if lm.cursor != last {
		t.Errorf("moveCursor(+1) at bottom moved cursor to %d, want %d (clamp)", lm.cursor, last)
	}
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
	path := filepath.Join(t.TempDir(), "f.yaml")
	if err := os.WriteFile(path, []byte(`a: 1
b: 2
c: 3
d: 4
e: 5
f: 6
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &followCfg{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	// Short viewport so the 6-line document overflows and can actually scroll.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = updated.(model)
	if m.preview.YOffset != 0 {
		t.Fatalf("initial YOffset = %d, want 0", m.preview.YOffset)
	}
	// Navigate down to "c" (third block, line 3).
	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	if it := m.list.SelectedItem(); it == nil || it.Key != "c" {
		t.Fatalf("selected = %v, want c", it)
	}
	if m.preview.YOffset != 2 {
		t.Errorf("YOffset following \"c\" = %d, want 2 (block at line 3)", m.preview.YOffset)
	}
}

// checkScreenInvariant asserts the two screen invariants that the enter* helpers
// are meant to guarantee:
//
//	m.alert != nil        ⟺  m.mode == paneAlert
//	len(m.blockEdits) > 0  ⟺  m.mode == paneBlockEdit
func checkScreenInvariant(t *testing.T, m model, where string) {
	t.Helper()
	if (m.alert != nil) != (m.mode == paneAlert) {
		t.Errorf("%s: alert/mode invariant broken: alert=%v mode=%d", where, m.alert != nil, m.mode)
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
	path := filepath.Join(t.TempDir(), "inv.yaml")
	if err := os.WriteFile(path, []byte(`server:
  host: localhost
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
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
	lm := newListModel([]string{"alpha", "beta", "gamma"}, nil, nil, 10)
	if lm.IsFiltering() {
		t.Fatal("should not start in filtering mode")
	}
	lm, _ = lm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !lm.IsFiltering() {
		t.Fatal("\"/\" should enter filtering mode")
	}
	for _, r := range "be" {
		lm, _ = lm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	got := lm.filteredItems()
	if len(got) != 1 || got[0].Key != "beta" {
		t.Errorf("filter \"be\" matched %v, want [beta]", got)
	}
}

// TestRootHintPanelToggle verifies that EnableHints opens the panel on start,
// "h" toggles it off and back on, and the field metadata is visible.
func TestRootHintPanelToggle(t *testing.T) {
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
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// EnableHints: true → panel visible on start.
	if !m.showHint {
		t.Fatal("EnableHints: true should show the hint panel on start")
	}
	view := m.View()
	if !strings.Contains(view, "Hint/Example") {
		t.Error("view should show the Hint/Example panel when EnableHints is true")
	}
	if !strings.Contains(view, "object") {
		t.Error("hint should show the selected field's type (server → object)")
	}

	// "h" hides the panel.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(model)
	if m.showHint {
		t.Fatal("pressing h should hide the hint panel")
	}
	if strings.Contains(m.View(), "Hint/Example") {
		t.Error("hint panel should be hidden after pressing h")
	}

	// "h" again shows it.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(model)
	if !m.showHint {
		t.Fatal("pressing h again should re-enable the hint panel")
	}
	if !strings.Contains(m.View(), "Hint/Example") {
		t.Error("hint panel should be visible after toggling back on")
	}
}

// TestReloadFromDisk covers ctrl+r in the main list: a clean document reloads
// immediately, a dirty one prompts first and reloads after confirmation.
func TestReloadFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reload.yaml")
	if err := os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Clean document: ctrl+r reloads immediately and picks up external changes.
	if err := os.WriteFile(path, []byte("server:\n  host: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(model)
	if m.mode != paneList {
		t.Fatalf("clean reload should not prompt; mode = %d", m.mode)
	}
	if !strings.Contains(string(m.doc.Raw()), "host: b") {
		t.Errorf("external change not loaded: %q", m.doc.Raw())
	}

	// Dirty document: ctrl+r prompts; confirming discards the local edit.
	if err := m.doc.Insert("extra: 1\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := os.WriteFile(path, []byte("server:\n  host: c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(model)
	if m.mode != paneAlert {
		t.Fatalf("dirty reload should prompt; mode = %d", m.mode)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("confirming the alert should produce a command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(model)
	if m.mode != paneList {
		t.Fatalf("expected list after confirmed reload; mode = %d", m.mode)
	}
	if !strings.Contains(string(m.doc.Raw()), "host: c") || strings.Contains(string(m.doc.Raw()), "extra") {
		t.Errorf("reload did not replace local state: %q", m.doc.Raw())
	}
	if m.doc.Dirty() {
		t.Error("reloaded document should not be dirty")
	}
}

// TestFilterAcceptsJK guards against j/k being swallowed as navigation while
// the filter input is active - filters like "unknown" (contains k) or
// "worker" (contains k) must be typeable; only the arrow keys navigate.
func TestFilterAcceptsJK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "filter.yaml")
	seed := "server:\n  host: a\nunknown-key: flagged\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(model)
	for _, r := range "unknown" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	if m.list.filter != "unknown" {
		t.Fatalf("filter = %q, want %q", m.list.filter, "unknown")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)

	sel := m.list.SelectedItem()
	if sel == nil || sel.Key != "unknown-key" {
		t.Fatalf("filter+enter selected %v, want unknown-key", sel)
	}
}
