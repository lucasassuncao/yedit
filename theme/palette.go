// Package theme provides the palette, base lipgloss styles, and shared
// layout primitives used across yedit-built TUIs.
package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// colorVal returns a lipgloss.Color unless NO_COLOR is set, in which case it
// returns an empty color (terminal default) so all rendering is monochrome.
func colorVal(c string) lipgloss.Color {
	if os.Getenv("NO_COLOR") != "" {
		return lipgloss.Color("")
	}
	return lipgloss.Color(c)
}

// Palette — narrow on purpose. Clients can extend it with their own colours;
// add to this list only when at least two yedit components need it.
var (
	Accent       = colorVal("63")  // blue — active borders, primary highlight
	AccentBright = colorVal("212") // pink — titles, selection
	Muted        = colorVal("240") // grey — inactive borders, status hints
	Dim          = colorVal("245") // light grey — secondary text
	Success      = colorVal("82")  // green — existing/added items, success alerts
	Danger       = colorVal("196") // red — error alerts
)

// Common item styles. Each TUI is free to compose its own variants on top.
var (
	SelectedItem  = lipgloss.NewStyle().Bold(true).Foreground(AccentBright)
	ExistingItem  = lipgloss.NewStyle().Foreground(Success)
	AvailableItem = lipgloss.NewStyle().Foreground(Dim)
	StatusBar     = lipgloss.NewStyle().Foreground(Muted).PaddingLeft(1)
)
