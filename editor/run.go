package editor

import (
	"fmt"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the editor TUI and blocks until the user quits. The Config must
// have Schema and Path set; everything else is optional.
//
// Returns nil on a clean quit, or the underlying tea.Program error. A panic
// inside the editor is recovered and returned as an error instead of crashing
// the embedding program: Bubble Tea restores the terminal before the panic
// propagates here, so the host is left with a usable terminal and a normal
// error to handle.
func Run(cfg Config) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yedit: editor panicked: %v\n%s", r, debug.Stack())
		}
	}()

	m, err := newModel(cfg)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
