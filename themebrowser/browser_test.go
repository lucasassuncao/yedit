package themebrowser

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucasassuncao/yedit/theme"
)

func TestBrowserModel_RendersAndNavigates(t *testing.T) {
	m := newBrowserModel(theme.ResolveColors(theme.Theme{}))

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Theme", "Category", "plain", "Miscellaneous"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should contain %q", want)
		}
	}

	// down moves the cursor.
	before := m.tbl.Cursor()
	updated, _ := m.Update(tea.KeyPressMsg{Text: "down", Code: tea.KeyDown})
	m = updated.(*browserModel)
	if m.tbl.Cursor() != before+1 {
		t.Errorf("cursor = %d, want %d after pressing down", m.tbl.Cursor(), before+1)
	}

	// up moves it back.
	updated, _ = m.Update(tea.KeyPressMsg{Text: "up", Code: tea.KeyUp})
	m = updated.(*browserModel)
	if m.tbl.Cursor() != before {
		t.Errorf("cursor = %d, want %d after pressing up", m.tbl.Cursor(), before)
	}
}

// TestBrowserModel_NotFullScreen guards the actual bug report: the view must
// not set tea.View.AltScreen (which clears/takes over the whole terminal
// just to show a small table).
func TestBrowserModel_NotFullScreen(t *testing.T) {
	m := newBrowserModel(theme.ResolveColors(theme.Theme{}))
	if v := m.View(); v.AltScreen {
		t.Error("View().AltScreen = true, want false - this should render inline, not take over the screen")
	}
}

// TestBrowserModel_NoOverflowTruncation guards the earlier bug: the box
// border must size itself to the table's actual content, never leaving a
// stray "…" from truncating a line that overflowed a manually-computed panel
// width.
func TestBrowserModel_NoOverflowTruncation(t *testing.T) {
	m := newBrowserModel(theme.ResolveColors(theme.Theme{}))
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "…") {
		t.Error("view contains a stray ellipsis - a line was truncated against a mismatched box width")
	}
}

// TestBrowserModel_SelectedRowHighlightSpansFullRow guards the compositing
// bug: the cursor row must be one uninterrupted background-highlighted span
// from the first column to the last. It previously reset partway through
// (at the boundary between the Theme and Category columns) because Cell had
// its own Foreground color - a self-contained ANSI reset baked into each
// cell - which cut off Selected's Background the moment the first cell's
// reset code fired, leaving only the first column highlighted.
func TestBrowserModel_SelectedRowHighlightSpansFullRow(t *testing.T) {
	m := newBrowserModel(theme.ResolveColors(theme.Theme{}))
	lines := strings.Split(m.View().Content, "\n")

	var selectedLine string
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "plain") {
			selectedLine = l
			break
		}
	}
	if selectedLine == "" {
		t.Fatal("could not find the row containing the initially-selected theme (plain)")
	}

	// Between the first column's text and the last column's text there must
	// be no reset sequence: a mid-row "\x1b[m" means the highlight broke and
	// restarted at the column boundary instead of spanning the row as one
	// block.
	start := strings.Index(selectedLine, "plain")
	end := strings.Index(selectedLine, "Miscellaneous")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("could not locate both column values in the selected row: %q", selectedLine)
	}
	if between := selectedLine[start:end]; strings.Contains(between, "\x1b[m") {
		t.Errorf("selected row resets its style between columns - the highlight does not span the full row:\n%q", between)
	}
}

func TestBrowserModel_QuitKey(t *testing.T) {
	m := newBrowserModel(theme.ResolveColors(theme.Theme{}))
	_, cmd := m.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if cmd == nil {
		t.Fatal("expected a Cmd for the quit key")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("quit key should produce tea.Quit, got %#v", msg)
	}
}
