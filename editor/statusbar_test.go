package editor

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// plainHelp returns a help.Model with no styling so lipgloss.Width measures
// raw character width, making test assertions predictable.
func plainHelp() help.Model {
	h := help.New()
	h.ShortSeparator = " • "
	plain := lipgloss.NewStyle()
	h.Styles.ShortKey = plain
	h.Styles.ShortDesc = plain
	h.Styles.ShortSeparator = plain
	h.Styles.Ellipsis = plain
	return h
}

// kb creates a key.Binding with a fixed display key and description.
func kb(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

type staticKeyMap []key.Binding

func (s staticKeyMap) ShortHelp() []key.Binding  { return []key.Binding(s) }
func (s staticKeyMap) FullHelp() [][]key.Binding { return nil }

func TestRenderLegend_FitsOnOneLine(t *testing.T) {
	h := plainHelp()
	km := staticKeyMap{kb("a", "alpha"), kb("b", "beta")}
	// "a alpha" (7) + " • " (3) + "b beta" (6) = 16
	_, lines := renderLegend(h, km, 80)
	if lines != 1 {
		t.Errorf("expected 1 line for wide terminal, got %d", lines)
	}
}

func TestRenderLegend_WrapsToTwoLines(t *testing.T) {
	h := plainHelp()
	// Each binding "ctrl+s save changes" = 19 chars; sep = 3; 4 bindings = 85 total
	km := staticKeyMap{
		kb("ctrl+s", "save changes"),
		kb("ctrl+u", "undo changes"),
		kb("ctrl+y", "redo changes"),
		kb("ctrl+l", "validate all"),
	}
	_, lines := renderLegend(h, km, 50)
	if lines < 2 {
		t.Errorf("expected at least 2 lines for narrow terminal, got %d", lines)
	}
}

func TestRenderLegend_ContentCorrect(t *testing.T) {
	h := plainHelp()
	km := staticKeyMap{kb("a", "alpha"), kb("b", "beta"), kb("c", "gamma")}
	// "a alpha" (7) + " • " (3) + "b beta" (6) = 16; "c gamma" (7) doesn't fit on width=18
	rendered, lines := renderLegend(h, km, 18)
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d (rendered: %q)", lines, rendered)
	}
	parts := strings.Split(rendered, "\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 newline-separated lines, got %d", len(parts))
	}
	if !strings.Contains(parts[0], "alpha") {
		t.Errorf("line 0 should contain 'alpha', got %q", parts[0])
	}
	if !strings.Contains(parts[1], "gamma") {
		t.Errorf("line 1 should contain 'gamma', got %q", parts[1])
	}
}

func TestRenderLegend_DisabledBindingsSkipped(t *testing.T) {
	h := plainHelp()
	disabled := kb("x", "hidden")
	disabled.SetEnabled(false)
	km := staticKeyMap{kb("a", "alpha"), disabled, kb("b", "beta")}
	rendered, lines := renderLegend(h, km, 80)
	if lines != 1 {
		t.Errorf("expected 1 line, got %d", lines)
	}
	if strings.Contains(rendered, "hidden") {
		t.Errorf("disabled binding should not appear in output, got %q", rendered)
	}
}

func TestRenderLegend_EmptyKeyMap(t *testing.T) {
	h := plainHelp()
	km := staticKeyMap{}
	rendered, lines := renderLegend(h, km, 80)
	if lines != 1 {
		t.Errorf("empty keymap should return 1 line, got %d", lines)
	}
	if rendered != "" {
		t.Errorf("empty keymap should return empty string, got %q", rendered)
	}
}

func TestRenderLegend_VeryNarrow(t *testing.T) {
	h := plainHelp()
	km := staticKeyMap{kb("a", "alpha"), kb("b", "beta"), kb("c", "gamma")}
	_, lines := renderLegend(h, km, 1)
	if lines != 3 {
		t.Errorf("expected 3 lines for width=1, got %d", lines)
	}
}
