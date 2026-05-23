package editor

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

// Short aliases over the shared palette to keep call sites tidy.
var (
	existingItemStyle  = theme.ExistingItem
	availableItemStyle = theme.AvailableItem
	selectedItemStyle  = theme.SelectedItem
	sectionLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).PaddingLeft(1)

	statusStyle       = theme.StatusBar
	filterPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentBright)

	overlayBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(theme.Accent).
				Padding(0, 1)
	overlayTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentBright)

	panelStyle       = theme.PanelBorder(false)
	activePanelStyle = theme.PanelBorder(true)
)

func renderHeader(title, file string, dirty bool, width int) string {
	info := file
	if dirty {
		info = file + " ● modified"
	}
	return theme.RenderHeader(title, info, "", width)
}
