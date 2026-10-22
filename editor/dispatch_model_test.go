package editor

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// dispatchModel opens a sized one-block document, reporting every model action.
func dispatchModel(t *testing.T, seen *[]ModelAction) model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dispatch.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{
		Path:   path,
		Schema: &sizeProbeConfig{},
		Trace:  Trace{OnModelAction: func(a ModelAction) { *seen = append(*seen, a) }},
	})
	require.NoError(t, err)
	u, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return u.(model)
}

func TestDispatchReportsTheActionToTheTrace(t *testing.T) {
	var seen []ModelAction
	m := dispatchModel(t, &seen)

	u, _ := m.dispatch(ToggleHints{})

	require.Equal(t, []ModelAction{ToggleHints{}}, seen)
	require.True(t, u.(model).showHint)
}

func TestDispatchDocUndoAndRedo(t *testing.T) {
	var seen []ModelAction
	m := dispatchModel(t, &seen)
	original := string(m.doc.Raw())
	var err error
	m.doc, err = m.doc.Insert("extra: 1\n")
	require.NoError(t, err)
	edited := string(m.doc.Raw())

	u, _ := m.dispatch(DocUndo{})
	require.Equal(t, original, string(u.(model).doc.Raw()))
	u, _ = u.(model).dispatch(DocRedo{})
	require.Equal(t, edited, string(u.(model).doc.Raw()))
}

// Replacing the whole document waits for a confirmation.
func TestDispatchApplyDocPresetAsksFirst(t *testing.T) {
	var seen []ModelAction
	m := dispatchModel(t, &seen)
	before := string(m.doc.Raw())

	u, _ := m.dispatch(ApplyDocPreset{Name: "basic", Content: "server:\n  host: b\n"})

	require.Equal(t, paneAlert, u.(model).mode)
	require.Equal(t, before, string(u.(model).doc.Raw()))
}

type unknownModelAction struct{}

func (unknownModelAction) modelAction() {}

func TestDispatchPanicsOnAnUnhandledAction(t *testing.T) {
	var seen []ModelAction
	m := dispatchModel(t, &seen)
	require.Panics(t, func() { m.dispatch(unknownModelAction{}) })
}
