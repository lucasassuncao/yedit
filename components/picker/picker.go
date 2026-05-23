// Package picker provides a compact list popover for choosing one item from
// a slice of strings. Designed to layer above a parent TUI.
package picker

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

// SelectedMsg fires when the user picks an item and confirms with Enter.
type SelectedMsg struct{ Name string }

// CancelledMsg fires when the user dismisses the picker with Esc.
type CancelledMsg struct{}

// Model is a compact list popover. The caller supplies the title shown at the
// top of the box, the candidate names, and an optional pre-selection.
type Model struct {
	title  string
	names  []string
	cursor int
	totalW int
	totalH int
}

// New creates a picker preselecting current if present in names; otherwise the
// first item is selected. Pass an empty title to render the picker without a
// title bar.
func New(title string, names []string, current string, totalW, totalH int) Model {
	cursor := 0
	for i, n := range names {
		if n == current {
			cursor = i
			break
		}
	}
	return Model{
		title:  title,
		names:  names,
		cursor: cursor,
		totalW: totalW,
		totalH: totalH,
	}
}

// Resize updates the centre region against which the picker is drawn.
func (p *Model) Resize(totalW, totalH int) {
	p.totalW = totalW
	p.totalH = totalH
}

// SelectedName returns the name of the currently-highlighted item, or "" if
// the cursor is out of range.
func (p Model) SelectedName() string {
	if p.cursor < 0 || p.cursor >= len(p.names) {
		return ""
	}
	return p.names[p.cursor]
}

// Update processes a message and returns the new model + any command.
// Non-key messages are ignored.
func (p Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	switch key.Type {
	case tea.KeyEsc:
		return p, func() tea.Msg { return CancelledMsg{} }
	case tea.KeyEnter:
		return p, func() tea.Msg { return SelectedMsg{Name: p.SelectedName()} }
	case tea.KeyUp:
		if p.cursor > 0 {
			p.cursor--
		}
	case tea.KeyDown:
		if p.cursor < len(p.names)-1 {
			p.cursor++
		}
	}
	return p, nil
}

// View renders the picker centred against totalW × totalH.
func (p Model) View() string {
	var lines []string
	if p.title != "" {
		lines = append(lines,
			lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Render(" "+p.title+" "),
			strings.Repeat("─", 20),
		)
	}
	for i, n := range p.names {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == p.cursor {
			prefix = "▸ "
			style = style.Foreground(theme.Accent).Bold(true)
		}
		lines = append(lines, style.Render(prefix+n))
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Faint(true).Render("[Enter] select  [Esc] cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	return theme.CenterBox(box, p.totalW, p.totalH)
}
