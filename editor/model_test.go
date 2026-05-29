package editor

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if m.mode != paneBlockEdit || m.blockEdit == nil {
		t.Fatalf("expected paneBlockEdit with non-nil blockEdit, got mode=%d blockEdit=%v", m.mode, m.blockEdit)
	}

	// Resize the terminal while the block editor is open.
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	m = updated.(model)

	if m.blockEdit == nil {
		t.Fatal("blockEdit went nil after resize")
	}
	if m.blockEdit.width != 120 || m.blockEdit.height != 50 {
		t.Errorf("block edit not resized: got %dx%d, want 120x50",
			m.blockEdit.width, m.blockEdit.height)
	}
}

// TestPreviewIsReadOnly verifies that typing in the preview pane never mutates
// the document — the right panel is a read-only, syntax-highlighted view.
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
	updated, _ = m.Update(blockEditCommittedMsg{Snippet: "server:\n  host: localhost\n"})
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

	// Type characters — a read-only preview must ignore them.
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
