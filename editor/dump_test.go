package editor

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/schema"
)

// readDumpEvents reads every JSONL line back into a generic map so tests can
// assert on individual fields without depending on dumpEvent's internals.
func readDumpEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var events []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var ev map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &ev))
		events = append(events, ev)
	}
	require.NoError(t, sc.Err())
	return events
}

func TestWireDump_CapturesKeyBlockAndModelEvents(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	path := filepath.Join(t.TempDir(), "trace.jsonl")
	d, err := newDumpWriter(path)
	must.NoError(err)

	cfg := Config{}
	wireDump(&cfg, d)

	cfg.Trace.OnMsg("list", tea.KeyMsg{Type: tea.KeyEnter})
	cfg.Trace.OnAction("server", ToggleField{NodeIdx: 2, Checked: true})
	cfg.Trace.OnModelAction(DrillOut{})
	must.NoError(d.close())

	events := readDumpEvents(t, path)
	must.Len(events, 3)

	is.Equal("key", events[0]["scope"])
	is.Equal("enter", events[0]["key"])
	is.Equal("list", events[0]["where"])

	is.Equal("block", events[1]["scope"])
	is.Equal("server", events[1]["where"])
	is.Equal("editor.ToggleField", events[1]["type"])

	is.Equal("model", events[2]["scope"])
	is.Equal("editor.DrillOut", events[2]["type"])

	// seq must be monotonic across all three scopes, not per-scope.
	is.EqualValues(1, events[0]["seq"])
	is.EqualValues(2, events[1]["seq"])
	is.EqualValues(3, events[2]["seq"])
}

func TestWireDump_CapturesGapMessages(t *testing.T) {
	// These messages previously had no representation anywhere in the dump:
	// they are handled directly in model.Update's switch (root.go) instead
	// of going through model.dispatch(ModelAction), so OnModelAction never
	// saw them. wireDump's broadened OnMsg must record all of them.
	is := assert.New(t)
	must := require.New(t)

	path := filepath.Join(t.TempDir(), "trace.jsonl")
	d, err := newDumpWriter(path)
	must.NoError(err)

	cfg := Config{}
	wireDump(&cfg, d)

	cfg.Trace.OnMsg("list", openItemMsg{Item: listItem{Key: "server"}})
	cfg.Trace.OnMsg("block:server:tree:editing", commitRequestedMsg{})
	cfg.Trace.OnMsg("list", confirmedDocPresetMsg{Name: "minimal", Content: "a: 1\n"})
	cfg.Trace.OnMsg("block:server:tree:editing", validateRequestedMsg{})
	must.NoError(d.close())

	events := readDumpEvents(t, path)
	must.Len(events, 4)
	for _, ev := range events {
		is.Equal("msg", ev["scope"])
	}
	is.Equal("editor.openItemMsg", events[0]["type"])
	is.Equal("editor.commitRequestedMsg", events[1]["type"])
	is.Equal("editor.confirmedDocPresetMsg", events[2]["type"])
	// exported fields on confirmedDocPresetMsg must survive the round trip.
	action, ok := events[2]["action"].(map[string]any)
	must.True(ok)
	is.Equal("minimal", action["Name"])
	is.Equal("editor.validateRequestedMsg", events[3]["type"])
}

func TestWireDump_FiltersNoise(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	path := filepath.Join(t.TempDir(), "trace.jsonl")
	d, err := newDumpWriter(path)
	must.NoError(err)

	cfg := Config{}
	wireDump(&cfg, d)

	cm := cursor.New()
	blinkCanceledCmd := cm.BlinkCmd()
	_ = cm.BlinkCmd() // cancels the context blinkCanceledCmd is waiting on

	cfg.Trace.OnMsg("block:server:tree:editing", cursor.BlinkMsg{})
	cfg.Trace.OnMsg("block:server:tree:editing", cursor.Blink())     // unexported cursor.initialBlinkMsg
	cfg.Trace.OnMsg("block:server:tree:editing", blinkCanceledCmd()) // unexported cursor.blinkCanceled
	cfg.Trace.OnMsg("list", clearStatusMsg{})
	cfg.Trace.OnMsg("list", tea.KeyMsg{Type: tea.KeyEnter}) // control: must still be recorded
	must.NoError(d.close())

	events := readDumpEvents(t, path)
	must.Len(events, 1, "blink and clear-status ticks must not reach the trace")
	is.Equal("key", events[0]["scope"])
}

func TestWireDump_PreservesExistingHooks(t *testing.T) {
	// Config.Dump must compose with caller-supplied OnAction/OnModelAction/
	// OnMsg rather than clobbering them.
	must := require.New(t)

	path := filepath.Join(t.TempDir(), "trace.jsonl")
	d, err := newDumpWriter(path)
	must.NoError(err)

	var gotAction, gotModelAction, gotMsg bool
	cfg := Config{
		Trace: Trace{
			OnAction:      func(string, BlockAction) { gotAction = true },
			OnModelAction: func(ModelAction) { gotModelAction = true },
			OnMsg:         func(string, tea.Msg) { gotMsg = true },
		},
	}
	wireDump(&cfg, d)

	cfg.Trace.OnAction("server", AddEntry{})
	cfg.Trace.OnModelAction(DrillOut{})
	cfg.Trace.OnMsg("list", tea.KeyMsg{Type: tea.KeyEnter})
	must.NoError(d.close())

	must.True(gotAction)
	must.True(gotModelAction)
	must.True(gotMsg)
}

func TestRedactModelAction_StripsDrillInDefsButKeepsIdentity(t *testing.T) {
	is := assert.New(t)

	big := make([]schema.FieldDef, 1000)
	redacted := redactModelAction(DrillIn{Key: "any", Defs: big, Kind: schema.KindList})

	di, ok := redacted.(DrillIn)
	is.True(ok)
	is.Equal("any", di.Key)
	is.Equal(schema.KindList, di.Kind)
	is.Nil(di.Defs, "Defs must be dropped so self-referential schemas cannot balloon the trace")

	// Non-DrillIn actions must pass through unchanged.
	is.Equal(DrillOut{}, redactModelAction(DrillOut{}))
}
