package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestTwoColumnWidthsFitsNarrowTerminals(t *testing.T) {
	for _, width := range []int{10, 20, 30, 43, 44, 80, 200} {
		listW, rightW := TwoColumnWidths(width)
		if listW < 1 || rightW < 1 {
			t.Errorf("TwoColumnWidths(%d) = (%d, %d), want both >= 1", width, listW, rightW)
		}
		if width >= 10 && listW+rightW+4 > width {
			t.Errorf("TwoColumnWidths(%d) = (%d, %d), total %d overflows the terminal",
				width, listW, rightW, listW+rightW+4)
		}
	}
}

func TestTwoColumnWidthsKeepsFloorsWhenWideEnough(t *testing.T) {
	listW, rightW := TwoColumnWidths(80)
	if listW != 30 {
		t.Errorf("TwoColumnWidths(80) listW = %d, want 30", listW)
	}
	if rightW != 80-30-4 {
		t.Errorf("TwoColumnWidths(80) rightW = %d, want %d", rightW, 80-30-4)
	}
}

func TestRenderTitledPanelClipsOverflowingContent(t *testing.T) {
	long := strings.Repeat("x", 50)
	content := strings.Repeat(long+"\n", 10)
	out := RenderTitledPanel("Title", Size{W: 20, H: 6}, false, content)

	lines := strings.Split(out, "\n")
	if len(lines) != 6 {
		t.Fatalf("panel height = %d lines, want 6", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 20 {
			t.Errorf("line %d width = %d, want 20", i, w)
		}
	}
}

func TestRenderHeaderTruncatesToWidth(t *testing.T) {
	out := RenderHeader(strings.Repeat("t", 30), "subtitle", "right-side", 20)
	if strings.Contains(out, "\n") {
		t.Fatal("header contains a newline, want a single line")
	}
	if w := lipgloss.Width(out); w > 20 {
		t.Errorf("header width = %d, want <= 20", w)
	}
}
