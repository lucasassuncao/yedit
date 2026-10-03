package editor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/yedit/spec"
)

// actionModel opens a one-block document with the given actions.
func actionModel(t *testing.T, actions []Action, validators ...Validator) (model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "action.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))
	m, err := newModel(Config{
		Path:          path,
		Schema:        &sizeProbeConfig{},
		NoSaveConfirm: true,
		Validators:    validators,
		Actions:       actions,
	})
	require.NoError(t, err)
	u, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return u.(model), path
}

// recorder is a Run that keeps every context it was handed.
type recorder struct {
	got []ActionContext
	res ActionResult
	err error
}

func (r *recorder) run(ctx ActionContext) (ActionResult, error) {
	r.got = append(r.got, ctx)
	return r.res, r.err
}

// edit makes the document dirty with content the file on disk does not have.
func edit(t *testing.T, m model) model {
	t.Helper()
	var err error
	m.doc, err = m.doc.ReplaceRaw([]byte("server:\n  host: b\n"))
	require.NoError(t, err)
	return m
}

// step feeds msg to m and returns what its command produced, batches drained.
func step(t *testing.T, m model, msg tea.Msg) (model, []tea.Msg) {
	t.Helper()
	u, cmd := m.Update(msg)
	if cmd == nil {
		return u.(model), nil
	}
	return u.(model), drainBatch(cmd())
}

// apply feeds msg to m and drops its command: a status message arms a timer
// that would make the test sleep.
func apply(m model, msg tea.Msg) model {
	u, _ := m.Update(msg)
	return u.(model)
}

// find returns the first message of type T in msgs.
func find[T any](t *testing.T, msgs []tea.Msg) T {
	t.Helper()
	for _, msg := range msgs {
		if v, ok := msg.(T); ok {
			return v
		}
	}
	var zero T
	t.Fatalf("no %T among %v", zero, msgs)
	return zero
}

func TestSaveFirstWritesThenRuns(t *testing.T) {
	must := require.New(t)
	r := &recorder{res: ActionResult{Message: "converted"}}
	m, path := actionModel(t, []Action{{Key: "ctrl+e", Help: "save & convert", SaveFirst: true, Run: r.run}})
	m = edit(t, m)

	m, msgs := step(t, m, actionRequestedMsg{key: "ctrl+e"})
	m, msgs = step(t, m, find[doSaveMsg](t, msgs))
	must.Empty(r.got, "the action must wait for the write")
	m, msgs = step(t, m, find[saveResultMsg](t, msgs))
	must.True(m.sh.IsBusy(), "the spinner runs while Run does")
	res := find[actionResultMsg](t, msgs)
	m, _ = step(t, m, res)

	must.False(m.sh.IsBusy())
	must.Len(r.got, 1)
	must.Equal(path, r.got[0].Path)
	got, err := os.ReadFile(path)
	must.NoError(err)
	must.Equal("server:\n  host: b\n", string(got), "Run saw a file that was not written yet")
	must.IsType(overlay.Alert{}, m.sh.TopOverlay())
}

func TestSaveFirstSkipsTheWriteWhenClean(t *testing.T) {
	r := &recorder{}
	m, _ := actionModel(t, []Action{{Key: "ctrl+e", Help: "convert", SaveFirst: true, Run: r.run}})

	_, msgs := step(t, m, actionRequestedMsg{key: "ctrl+e"})

	find[actionResultMsg](t, msgs)
	require.Len(t, r.got, 1)
}

func TestSaveFirstDoesNotRunWhenValidationFails(t *testing.T) {
	failing := spec.ValidatorFunc(func(spec.ValidationInput) []spec.Violation {
		return []spec.Violation{{Path: "server", Message: "broken"}}
	})
	r := &recorder{}
	m, _ := actionModel(t, []Action{{Key: "ctrl+e", Help: "convert", SaveFirst: true, Run: r.run}}, failing)
	m = edit(t, m)

	m, msgs := step(t, m, actionRequestedMsg{key: "ctrl+e"})

	require.Empty(t, msgs, "nothing may be written or run past a validation error")
	require.Equal(t, paneAlert, m.mode)
	require.Empty(t, r.got)
}

func TestWithoutSaveFirstRunsOnTheUnsavedDocument(t *testing.T) {
	must := require.New(t)
	r := &recorder{}
	m, path := actionModel(t, []Action{{Key: "ctrl+o", Help: "dry run", Run: r.run}})
	m = edit(t, m)

	_, msgs := step(t, m, actionRequestedMsg{key: "ctrl+o"})

	find[actionResultMsg](t, msgs)
	must.Len(r.got, 1)
	must.Equal("server:\n  host: b\n", string(r.got[0].Raw), "Run gets the document as shown")
	got, err := os.ReadFile(path)
	must.NoError(err)
	must.Equal("server:\n  host: a\n", string(got), "nothing was saved")
}

func TestASecondActionWaitsForTheFirst(t *testing.T) {
	r := &recorder{}
	m, _ := actionModel(t, []Action{{Key: "ctrl+o", Help: "dry run", Run: r.run}})
	m, _ = step(t, m, actionRequestedMsg{key: "ctrl+o"})
	require.True(t, m.sh.IsBusy())

	m = apply(m, actionRequestedMsg{key: "ctrl+o"})

	require.Len(t, r.got, 1, "the second press must not start another run")
	require.Contains(t, m.sh.Status(), "still running")
}

func TestOutputOpensAPager(t *testing.T) {
	r := &recorder{res: ActionResult{Message: "3 changes", Output: strings.Repeat("would install x\n", 80)}}
	m, _ := actionModel(t, []Action{{Key: "ctrl+o", Help: "dry run", Run: r.run}})

	m, msgs := step(t, m, actionRequestedMsg{key: "ctrl+o"})
	m, _ = step(t, m, find[actionResultMsg](t, msgs))

	require.IsType(t, overlay.Pager{}, m.sh.TopOverlay())
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "dry run")
	require.Contains(t, view, "3 changes")
}

func TestFailureKeepsItsOutput(t *testing.T) {
	r := &recorder{res: ActionResult{Output: "step 2: permission denied"}, err: errors.New("exit status 1")}
	m, _ := actionModel(t, []Action{{Key: "ctrl+o", Help: "install", Run: r.run}})

	m, msgs := step(t, m, actionRequestedMsg{key: "ctrl+o"})
	m, _ = step(t, m, find[actionResultMsg](t, msgs))

	require.IsType(t, overlay.Pager{}, m.sh.TopOverlay())
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "install failed")
	require.Contains(t, view, "permission denied")
}

func TestExecReloadsACleanDocumentItChanged(t *testing.T) {
	must := require.New(t)
	m, path := actionModel(t, []Action{{Key: "ctrl+x", Help: "open in $EDITOR"}})
	must.NoError(os.WriteFile(path, []byte("server:\n  host: from-editor\n"), 0o600))

	m, msgs := step(t, m, actionExecDoneMsg{help: "open in $EDITOR"})
	m = apply(m, find[reloadResultMsg](t, msgs))

	must.Equal("server:\n  host: from-editor\n", string(m.doc.Raw()))
}

func TestExecKeepsUnsavedEditsOverAChangedFile(t *testing.T) {
	must := require.New(t)
	m, path := actionModel(t, []Action{{Key: "ctrl+x", Help: "open in $EDITOR"}})
	m = edit(t, m)
	must.NoError(os.WriteFile(path, []byte("server:\n  host: from-editor\n"), 0o600))

	m = apply(m, actionExecDoneMsg{help: "open in $EDITOR"})

	must.Equal("server:\n  host: b\n", string(m.doc.Raw()), "unsaved edits must survive")
	must.Contains(m.sh.Status(), "ctrl+r")
}

func TestExecReportsAFailedProgram(t *testing.T) {
	m, _ := actionModel(t, []Action{{Key: "ctrl+x", Help: "open in $EDITOR"}})

	m, _ = step(t, m, actionExecDoneMsg{help: "open in $EDITOR", err: errors.New("exit status 2")})

	require.Equal(t, paneAlert, m.mode)
	require.Contains(t, ansi.Strip(m.View().Content), "open in $EDITOR failed")
}

func TestActionsAreInTheLegendAndBoundToTheirKey(t *testing.T) {
	m, _ := actionModel(t, []Action{{Key: "ctrl+e", Help: "save & convert", SaveFirst: true, Run: (&recorder{}).run}})

	require.Contains(t, ansi.Strip(m.View().Content), "save & convert")
	_, msgs := step(t, m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
	require.Equal(t, []tea.Msg{actionRequestedMsg{key: "ctrl+e"}}, msgs)
}
