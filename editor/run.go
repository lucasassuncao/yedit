package editor

import (
	"context"
	"fmt"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
)

// Result reports the outcome of an editor session.
type Result struct {
	// Saved is true when at least one save to disk succeeded during the
	// session. It stays true even if the user keeps editing afterwards and
	// quits with unsaved changes.
	Saved bool
}

// Run starts the editor TUI and blocks until the user quits. The Config must
// have Schema and Path set; everything else is optional.
//
// Returns the session Result on a clean quit, or the underlying tea.Program
// error. A panic inside the editor is recovered and returned as an error
// instead of crashing the embedding program: Bubble Tea restores the terminal
// before the panic propagates here, so the host is left with a usable
// terminal and a normal error to handle.
func Run(cfg Config) (Result, error) {
	return RunContext(context.Background(), cfg)
}

// NewModelForTest constructs the editor model for use in external test packages.
// In production code use Run or RunContext; this entry point skips the bubbletea
// program and returns the raw tea.Model so tests can drive it via Update/View.
func NewModelForTest(cfg Config) (tea.Model, error) {
	return newModel(cfg)
}

// RunContext is Run with a context: cancelling ctx shuts the editor down and
// makes RunContext return the context's error. Unsaved changes are discarded
// on cancellation, but Result.Saved still reports any save that completed
// before it.
func RunContext(ctx context.Context, cfg Config) (res Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("yamltui: editor panicked: %v\n%s", r, debug.Stack())
		}
	}()

	m, err := newModel(cfg)
	if err != nil {
		return Result{}, err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := p.Run()
	if fm, ok := final.(model); ok {
		res.Saved = fm.saved
	}
	return res, err
}
