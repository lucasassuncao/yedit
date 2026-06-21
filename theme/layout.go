package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Size holds a width/height pair. Used wherever a terminal or panel dimension
// is passed as a unit (picker, RenderTitledPanel).
type Size struct{ W, H int }

// ClampScroll adjusts a scroll offset so that the cursor row stays within the
// visible window of height rows, and returns the corrected offset. It scrolls
// up when the cursor sits above the window and down when it sits below, and
// never returns a negative offset. A non-positive height leaves the offset
// untouched (apart from the floor at zero).
func ClampScroll(cursor, offset, height int) int {
	if cursor < offset {
		offset = cursor
	}
	if height > 0 && cursor >= offset+height {
		offset = cursor - height + 1
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// TwoColumnLayout carries the five sections of the standard two-panel screen.
type TwoColumnLayout struct {
	Header   string
	Left     string
	Right    string
	Feedback string // pass "" when there is nothing to report
	Legend   string // key/action legend line
}

// RenderHeader returns a single-line header. title is rendered bold on the
// left, subtitle (if non-empty) follows after a separator, right (if non-empty)
// is right-aligned for context such as filenames.
func RenderHeader(title, subtitle, right string, width int) string {
	return RenderHeaderWith(title, subtitle, right, width, Colors{
		SelectionColor:     string(AccentBright),
		AvailableItemColor: string(Dim),
	})
}

// TwoColumnWidths computes left and right column widths for the standard
// two-panel layout: left is totalWidth/3, clamped to [30, 60]; right gets
// the remainder minus 4 chars for the two border pairs.
func TwoColumnWidths(totalWidth int) (listW, rightW int) {
	listW = totalWidth / 3
	if listW < 30 {
		listW = 30
	}
	if listW > 60 {
		listW = 60
	}
	rightW = totalWidth - listW - 4
	if rightW < 10 {
		rightW = 10
	}
	return
}

// RenderTwoColumnView assembles the standard two-panel screen: header, panels
// side by side, a feedback line, and a legend line.
func RenderTwoColumnView(layout TwoColumnLayout) string {
	body := lipgloss.JoinHorizontal(lipgloss.Top, layout.Left, layout.Right)
	return strings.Join([]string{layout.Header, body, layout.Feedback, layout.Legend}, "\n")
}

// RenderTitledPanel renders a rounded-border panel with the title embedded in
// the top edge: ╭─ Title ──────╮. size holds the OUTER dimensions (including
// the border rows/cols).
func RenderTitledPanel(title string, size Size, active bool, content string) string {
	return RenderTitledPanelWith(title, size, active, content, Colors{
		ActiveBorderColor:   string(Accent),
		InactiveBorderColor: string(Muted),
		SelectionColor:      string(AccentBright),
		AvailableItemColor:  string(Dim),
	})
}

// RenderTitledPanelWith is like RenderTitledPanel but derives border and title
// colors from c instead of the package-level palette vars.
func RenderTitledPanelWith(title string, size Size, active bool, content string, c Colors) string {
	width, height := size.W, size.H
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}

	var borderColor, titleColor lipgloss.Color
	if active {
		borderColor = lipgloss.Color(c.ActiveBorderColor)
		titleColor = lipgloss.Color(c.SelectionColor)
	} else {
		borderColor = lipgloss.Color(c.InactiveBorderColor)
		titleColor = lipgloss.Color(c.AvailableItemColor)
	}

	innerW := width - 2
	titleSegment := lipgloss.NewStyle().Bold(true).Foreground(titleColor).Render(" " + title + " ")
	fillLen := innerW - 1 - lipgloss.Width(titleSegment)
	if fillLen < 0 {
		fillLen = 0
	}

	borderInk := lipgloss.NewStyle().Foreground(borderColor)
	top := borderInk.Render("╭─") + titleSegment + borderInk.Render(strings.Repeat("─", fillLen)+"╮")

	body := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(borderColor).
		Width(innerW).
		Height(height - 2).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, top, body)
}

// RenderHeaderWith is like RenderHeader but derives title and info colors from
// c instead of the package-level palette vars.
func RenderHeaderWith(title, subtitle, right string, width int, c Colors) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c.SelectionColor)).PaddingLeft(1)
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(c.AvailableItemColor)).PaddingRight(1)

	left := titleStyle.Render(title)
	if subtitle != "" {
		left += infoStyle.Render(" · " + subtitle)
	}
	rightRendered := ""
	if right != "" {
		rightRendered = infoStyle.Render(right)
	}
	spacerW := width - lipgloss.Width(left) - lipgloss.Width(rightRendered)
	if spacerW < 1 {
		spacerW = 1
	}
	return left + strings.Repeat(" ", spacerW) + rightRendered
}

// Composite overlays fg on top of bg at position (x, y). For each line in fg,
// the corresponding bg line has its (x … x+fgW) segment replaced by the fg
// line, preserving ANSI color sequences in both strings.
func Composite(fg, bg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		bgRow := y + i
		if bgRow < 0 || bgRow >= len(bgLines) {
			continue
		}
		bgLine := bgLines[bgRow]

		left := ansi.Truncate(bgLine, x, "")
		leftW := ansi.StringWidth(left)
		if leftW < x {
			left += strings.Repeat(" ", x-leftW)
		}

		fgW := ansi.StringWidth(fgLine)
		right := ansi.TruncateLeft(bgLine, x+fgW, "")

		bgLines[bgRow] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

// CompositeCenter centers fg over bg, replacing the bg cells behind it.
func CompositeCenter(fg, bg string) string {
	bgLines := strings.Split(bg, "\n")
	bgH := len(bgLines)
	bgW := 0
	for _, l := range bgLines {
		if w := ansi.StringWidth(l); w > bgW {
			bgW = w
		}
	}

	fgLines := strings.Split(fg, "\n")
	fgH := len(fgLines)
	fgW := 0
	for _, l := range fgLines {
		if w := ansi.StringWidth(l); w > fgW {
			fgW = w
		}
	}

	x := (bgW - fgW) / 2
	y := (bgH - fgH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return Composite(fg, bg, x, y)
}
