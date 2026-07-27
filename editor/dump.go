package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
)

// dumpWriter records every action and keystroke of a session to a JSONL file, so
// a bug report can be replayed later.
type dumpWriter struct {
	f   *os.File
	enc *json.Encoder
	seq int
}

// newDumpWriter creates the dump file at path, or a timestamped file in the OS
// temp dir when path is empty.
func newDumpWriter(path string) (*dumpWriter, error) {
	if path == "" {
		path = filepath.Join(os.TempDir(), fmt.Sprintf("yedit-dump-%d.jsonl", time.Now().UnixNano()))
	}
	f, err := os.Create(path) // #nosec G304 -- path is supplied by the embedding application (Config.Trace.DumpPath) or generated internally
	if err != nil {
		return nil, err
	}
	return &dumpWriter{f: f, enc: json.NewEncoder(f)}, nil
}

// dumpEvent is one line of the session dump. Declaration order is the JSON key
// order, which a map would not preserve.
type dumpEvent struct {
	TS     time.Time `json:"ts"`
	Seq    int       `json:"seq"`
	Scope  string    `json:"scope"`
	Where  string    `json:"where"`
	Key    string    `json:"key,omitempty"`
	Type   string    `json:"type,omitempty"`
	Action any       `json:"action,omitempty"`
}

// writeAction appends one action event. scope is "model" or "block"; where is
// the block key, empty for "model".
func (d *dumpWriter) writeAction(scope, where string, action any) {
	d.seq++
	_ = d.enc.Encode(dumpEvent{
		TS:     time.Now(),
		Seq:    d.seq,
		Scope:  scope,
		Where:  where,
		Type:   fmt.Sprintf("%T", action),
		Action: action,
	})
}

// writeKey appends one keystroke event, with key the human-readable name
// (e.g. "enter", "ctrl+c").
func (d *dumpWriter) writeKey(where, key string) {
	d.seq++
	_ = d.enc.Encode(dumpEvent{
		TS:    time.Now(),
		Seq:   d.seq,
		Scope: "key",
		Where: where,
		Key:   key,
	})
}

// writeMsg appends one raw tea.Msg event. tea.KeyMsg goes to the "key" scope;
// everything else lands under "msg", including the internal messages driving
// commit/save, confirmations, and validation, which dispatch no action of their
// own - so the trace has no gaps. Messages with only unexported fields still
// record their type name even though Action serializes to "{}".
func (d *dumpWriter) writeMsg(where string, msg tea.Msg) {
	if km, ok := msg.(tea.KeyMsg); ok {
		d.writeKey(where, km.String())
		return
	}
	d.seq++
	_ = d.enc.Encode(dumpEvent{
		TS:     time.Now(),
		Seq:    d.seq,
		Scope:  "msg",
		Where:  where,
		Type:   fmt.Sprintf("%T", msg),
		Action: msg,
	})
}

// isDumpNoise reports whether msg is a high-frequency internal message that says
// nothing about what the user did and would flood the trace: cursor blinks and
// yedit's own status-bar decay timer.
//
// cursor's initialBlinkMsg and blinkCanceled are unexported, so they cannot be
// named in a type switch and are matched by their %T name instead.
func isDumpNoise(msg tea.Msg) bool {
	switch msg.(type) {
	case cursor.BlinkMsg, clearStatusMsg:
		return true
	}
	switch fmt.Sprintf("%T", msg) {
	case "cursor.initialBlinkMsg", "cursor.blinkCanceled":
		return true
	default:
		return false
	}
}

// redactModelAction strips static schema metadata before an action is dumped.
// DrillIn.Defs is the worst offender: the fully expanded schema subtree, which
// for self-referential types balloons to megabytes per event while never varying
// with user input.
func redactModelAction(a ModelAction) ModelAction {
	if di, ok := a.(DrillIn); ok {
		return DrillIn{Key: di.Key, Kind: di.Kind, RelSegs: di.RelSegs}
	}
	return a
}

func (d *dumpWriter) path() string { return d.f.Name() }
func (d *dumpWriter) close() error { return d.f.Close() }

// wireDump composes cfg.Trace's hooks with d, preserving any the caller already
// set so Config.Trace.Dump and manual hooks work together.
func wireDump(cfg *Config, d *dumpWriter) {
	prevAction := cfg.Trace.OnAction
	cfg.Trace.OnAction = func(blockKey string, a BlockAction) {
		d.writeAction("block", blockKey, a)
		if prevAction != nil {
			prevAction(blockKey, a)
		}
	}

	prevModelAction := cfg.Trace.OnModelAction
	cfg.Trace.OnModelAction = func(a ModelAction) {
		d.writeAction("model", "", redactModelAction(a))
		if prevModelAction != nil {
			prevModelAction(a)
		}
	}

	prevMsg := cfg.Trace.OnMsg
	cfg.Trace.OnMsg = func(where string, msg tea.Msg) {
		if !isDumpNoise(msg) {
			d.writeMsg(where, msg)
		}
		if prevMsg != nil {
			prevMsg(where, msg)
		}
	}
}
