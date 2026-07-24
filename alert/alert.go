// Package alert provides a modal alert/confirm component for bubbletea TUIs.
//
// Use New for an informational modal with a single OK button, and NewConfirm
// for a Yes/No prompt that runs a tea.Cmd on confirmation.
package alert

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/theme"
)

// DismissedMsg is sent when the user closes the alert.
type DismissedMsg struct{}

// Kind discriminates the alert flavour. Each kind changes the accent colour
// and the available actions.
type Kind int

const (
	KindError   Kind = iota // red border, OK button
	KindSuccess             // green border, OK button
	KindWarning             // orange border, OK button
	KindConfirm             // accent border, Yes/No buttons
)

// Model is a centred modal that overlays a parent TUI.
//
// confirmYes is only meaningful when Kind == KindConfirm.
// confirmCmd is the tea.Cmd dispatched when the user confirms (Yes / Enter
// while Yes is highlighted / y).
type Model struct {
	title      string
	lines      []string
	kind       Kind
	confirmYes bool
	confirmCmd tea.Cmd
}

// New builds an informational modal with a single OK button.
func New(title, message string, kind Kind) Model {
	return Model{
		title: title,
		lines: strings.Split(message, "\n"),
		kind:  kind,
	}
}

// NewConfirm builds a Yes/No modal that runs confirmCmd when the user picks
// Yes. Yes is the default focus.
func NewConfirm(title, message string, confirmCmd tea.Cmd) Model {
	return Model{
		title:      title,
		lines:      strings.Split(message, "\n"),
		kind:       KindConfirm,
		confirmYes: true,
		confirmCmd: confirmCmd,
	}
}

func (a Model) accentColor() color.Color {
	switch a.kind {
	case KindSuccess:
		return theme.Success
	case KindWarning:
		return theme.Warning
	case KindConfirm:
		return theme.Accent
	default:
		return theme.Danger
	}
}

// Update processes a key event and returns the new model and any command.
// Non-key messages are ignored (the parent decides what reaches the modal).
func (a Model) Update(msg tea.KeyMsg) (Model, tea.Cmd) {
	if a.kind == KindConfirm {
		switch msg.String() {
		case "left", "h", "right", "l", "tab":
			a.confirmYes = !a.confirmYes
		case "y", "Y":
			return a, a.confirm()
		case "n", "N", "esc", "q":
			return a, func() tea.Msg { return DismissedMsg{} }
		case "enter", " ":
			if a.confirmYes {
				return a, a.confirm()
			}
			return a, func() tea.Msg { return DismissedMsg{} }
		}
		return a, nil
	}
	switch msg.String() {
	case " ", "enter", "esc", "q":
		return a, func() tea.Msg { return DismissedMsg{} }
	}
	return a, nil
}

// confirm returns the command to run when the user picks Yes. A nil
// confirmCmd falls back to dismissing the modal so the confirm key is never
// silently eaten.
func (a Model) confirm() tea.Cmd {
	if a.confirmCmd != nil {
		return a.confirmCmd
	}
	return func() tea.Msg { return DismissedMsg{} }
}

// Box renders the modal box without any positioning.
// The caller is responsible for compositing it over the background view.
func (a Model) Box() string {
	color := a.accentColor()

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	title := titleStyle.Render(a.title)

	// Measure in terminal cells (not bytes) so multibyte runes and symbols do
	// not skew the OK-button centering.
	maxW := 0
	for _, l := range a.lines {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}

	body := strings.Join(a.lines, "\n")

	btnStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("231")).
		Background(color).
		Padding(0, 2)

	var buttons string
	if a.kind == KindConfirm {
		inactiveStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(theme.Muted).
			Padding(0, 2)
		yesStyle, noStyle := inactiveStyle, inactiveStyle
		if a.confirmYes {
			yesStyle = btnStyle
		} else {
			noStyle = btnStyle
		}
		yes := yesStyle.Render("  Yes  ")
		no := noStyle.Render("  No  ")
		buttons = lipgloss.JoinHorizontal(lipgloss.Top, yes, "  ", no)
	} else {
		ok := btnStyle.Render("  OK  ")
		buttons = lipgloss.NewStyle().Width(maxW).Align(lipgloss.Center).Render(ok)
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(1, 3)

	return border.Render(strings.Join([]string{title, "", body, "", buttons}, "\n"))
}
