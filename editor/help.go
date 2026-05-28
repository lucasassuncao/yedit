package editor

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

type helpEntry struct {
	key  string
	desc string
}

type helpSection struct {
	name    string
	entries []helpEntry
}

// renderHelpOverlay renders a centred floating help box.
func renderHelpOverlay(sections []helpSection, term theme.Size) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentBright)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	keyStyle := lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(theme.Dim)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Keyboard Shortcuts") + "\n\n")
	for _, s := range sections {
		sb.WriteString(sectionStyle.Render(s.name) + "\n")
		for _, e := range s.entries {
			sb.WriteString("  " + keyStyle.Render(e.key))
			sb.WriteString("  " + descStyle.Render(e.desc) + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(descStyle.Render("[?] close help"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 3).
		Render(sb.String())

	return theme.CenterBox(box, term)
}

var listHelpSections = []helpSection{
	{
		name: "Navigation",
		entries: []helpEntry{
			{key: "j / ↓", desc: "move down"},
			{key: "k / ↑", desc: "move up"},
			{key: "g", desc: "jump to top"},
			{key: "G", desc: "jump to bottom"},
			{key: "/", desc: "filter list"},
		},
	},
	{
		name: "Actions",
		entries: []helpEntry{
			{key: "Enter", desc: "open or add block"},
			{key: "ctrl+d", desc: "delete block"},
			{key: "ctrl+u", desc: "undo last change"},
			{key: "Tab", desc: "switch to YAML editor"},
			{key: "ctrl+s", desc: "save file"},
			{key: "ctrl+l", desc: "validate document"},
			{key: "Esc / q", desc: "quit (prompts if dirty)"},
		},
	},
}

var blockHelpSections = []helpSection{
	{
		name: "Navigation",
		entries: []helpEntry{
			{key: "j / ↓", desc: "move down"},
			{key: "k / ↑", desc: "move up"},
			{key: "g", desc: "jump to top"},
			{key: "G", desc: "jump to bottom"},
			{key: "→ / l", desc: "expand node"},
			{key: "← / h", desc: "collapse node"},
		},
	},
	{
		name: "Actions",
		entries: []helpEntry{
			{key: "Space", desc: "toggle field on/off"},
			{key: "Enter", desc: "expand/collapse • add seq item"},
			{key: "ctrl+d", desc: "delete sequence item"},
			{key: "p", desc: "open preset picker"},
			{key: "Tab", desc: "switch to YAML editor"},
			{key: "ctrl+s", desc: "commit changes to document"},
			{key: "Esc", desc: "back (warns if uncommitted)"},
		},
	},
}
