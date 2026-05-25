// Package theme provides the palette, base lipgloss styles, and shared
// layout primitives used across yedit-built TUIs.
package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette — narrow on purpose. Clients can extend it with their own colours;
// add to this list only when at least two yedit components need it.
var (
	Accent       = lipgloss.Color("63")  // blue — active borders, primary highlight
	AccentBright = lipgloss.Color("212") // pink — titles, selection
	Muted        = lipgloss.Color("240") // grey — inactive borders, status hints
	Dim          = lipgloss.Color("245") // light grey — secondary text
	Success      = lipgloss.Color("82")  // green — existing/added items, success alerts
	Warning      = lipgloss.Color("214") // orange — dirty marker
	Danger       = lipgloss.Color("196") // red — error alerts
)

// Common item styles. Each TUI is free to compose its own variants on top.
var (
	SelectedItem  = lipgloss.NewStyle().Bold(true).Foreground(AccentBright)
	ExistingItem  = lipgloss.NewStyle().Foreground(Success)
	AvailableItem = lipgloss.NewStyle().Foreground(Dim)
	StatusBar     = lipgloss.NewStyle().Foreground(Muted).PaddingLeft(1)
)

var (
	headerTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(AccentBright).PaddingLeft(1)
	headerInfoStyle  = lipgloss.NewStyle().Foreground(Dim).PaddingRight(1)
)

// RenderHeader returns a single-line header. title is rendered bold on the
// left, subtitle (if non-empty) follows after a separator, right (if non-empty)
// is right-aligned for context such as filenames.
func RenderHeader(title, subtitle, right string, width int) string {
	left := headerTitleStyle.Render(title)
	if subtitle != "" {
		left += headerInfoStyle.Render(" · " + subtitle)
	}
	rightRendered := ""
	if right != "" {
		rightRendered = headerInfoStyle.Render(right)
	}
	spacerW := width - lipgloss.Width(left) - lipgloss.Width(rightRendered)
	if spacerW < 1 {
		spacerW = 1
	}
	return left + strings.Repeat(" ", spacerW) + rightRendered
}

// PanelBorder returns a rounded-border style coloured for the active/inactive
// state. Width/Height are left to the caller because layout differs per TUI.
func PanelBorder(active bool) lipgloss.Style {
	colour := Muted
	if active {
		colour = Accent
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colour)
}

// TwoColumnWidths computes left and right column widths for the standard
// two-panel layout: left is totalWidth/5 (min 40); right gets the remainder
// minus 4 chars for the two border pairs.
func TwoColumnWidths(totalWidth int) (listW, rightW int) {
	listW = totalWidth / 5
	if listW < 40 {
		listW = 40
	}
	rightW = totalWidth - listW - 4
	if rightW < 10 {
		rightW = 10
	}
	return
}

// Size holds a width/height pair. Used wherever a terminal or panel dimension
// is passed as a unit (alert, picker, RenderTitledPanel, CenterBox).
type Size struct{ W, H int }

// TwoColumnLayout carries the five sections of the standard two-panel screen.
type TwoColumnLayout struct {
	Header   string
	Left     string
	Right    string
	Feedback string // pass "" when there is nothing to report
	Hint     string
}

// RenderTwoColumnView assembles the standard two-panel screen: header, panels
// side by side, a feedback line, and a hint line.
func RenderTwoColumnView(layout TwoColumnLayout) string {
	body := lipgloss.JoinHorizontal(lipgloss.Top, layout.Left, layout.Right)
	return strings.Join([]string{layout.Header, body, layout.Feedback, layout.Hint}, "\n")
}

// RenderTitledPanel renders a rounded-border panel with the title embedded in
// the top edge: ╭─ Title ──────╮. size holds the OUTER dimensions (including
// the border rows/cols).
func RenderTitledPanel(title string, size Size, active bool, content string) string {
	width, height := size.W, size.H
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}

	borderColor := Muted
	titleColor := Dim
	if active {
		borderColor = Accent
		titleColor = AccentBright
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

// CenterBox positions box at the centre of the given terminal Size by adding
// padding. Used by floating overlay/alert/picker views.
func CenterBox(box string, term Size) string {
	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	lp := (term.W - bw) / 2
	tp := (term.H - bh) / 2
	if lp < 0 {
		lp = 0
	}
	if tp < 0 {
		tp = 0
	}
	return lipgloss.NewStyle().PaddingLeft(lp).PaddingTop(tp).Render(box)
}
