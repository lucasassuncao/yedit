package viewer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// tallSource returns a preset far taller than any test viewport so the right
// pane has something to scroll.
type tallSource struct{}

func (tallSource) ListFields() []string        { return []string{"alpha", "beta"} }
func (tallSource) ListPresets(string) []string { return []string{"base"} }
func (tallSource) PresetYAML(f, n string) (string, error) {
	return strings.Repeat("key: value\n", 100), nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgdn":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newTallModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(tallSource{})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return &m
}

func TestViewportPaneScrolls(t *testing.T) {
	m := newTallModel(t)
	m.Update(key("tab")) // focus the right pane
	if m.active != paneViewport {
		t.Fatal("tab should focus the viewport pane")
	}

	m.Update(key("down"))
	if m.vp.YOffset != 1 {
		t.Errorf("YOffset after down = %d, want 1", m.vp.YOffset)
	}

	m.Update(key("pgdn"))
	if m.vp.YOffset <= 1 {
		t.Errorf("YOffset after pgdn = %d, want > 1", m.vp.YOffset)
	}

	m.Update(key("up"))
	after := m.vp.YOffset
	m.Update(key("up"))
	if m.vp.YOffset >= after && after > 0 {
		t.Errorf("up did not scroll back (offset %d -> %d)", after, m.vp.YOffset)
	}
}

func TestViewportScrollResetsOnSelectionChange(t *testing.T) {
	m := newTallModel(t)
	m.Update(key("tab"))
	m.Update(key("pgdn"))
	if m.vp.YOffset == 0 {
		t.Fatal("precondition: viewport should be scrolled")
	}

	m.Update(key("tab"))  // back to the list
	m.Update(key("down")) // select another field
	if m.vp.YOffset != 0 {
		t.Errorf("YOffset after selection change = %d, want 0", m.vp.YOffset)
	}
}

func TestListPaneIgnoresViewportKeysWhenUnfocused(t *testing.T) {
	m := newTallModel(t)
	m.Update(key("down")) // list focused: moves the field cursor
	f, _ := m.list.Selected()
	if f != "beta" {
		t.Errorf("field after down = %q, want beta", f)
	}
	if m.vp.YOffset != 0 {
		t.Errorf("viewport scrolled while list focused: YOffset = %d", m.vp.YOffset)
	}
}
