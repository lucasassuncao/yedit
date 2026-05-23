package editor

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the editor TUI and blocks until the user quits. The Config must
// have Schema and Path set; everything else is optional.
//
// Returns nil on a clean quit, or the underlying tea.Program error.
func Run(cfg Config) error {
	m, err := newModel(cfg)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
