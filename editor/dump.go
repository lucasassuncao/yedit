package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
)

// dumpWriter records every BlockAction/ModelAction/keystroke dispatched
// during a session to a JSONL file, so a bug report can be replayed later.
type dumpWriter struct {
	f   *os.File
	enc *json.Encoder
	seq int
}

// newDumpWriter creates the dump file at path and returns a writer for it.
// An empty path falls back to a timestamped file in the OS temp dir.
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

// dumpEvent is one line of the session dump. Field order matches the JSON
// key order (structs preserve declaration order; a map would not).
type dumpEvent struct {
	TS     time.Time `json:"ts"`
	Seq    int       `json:"seq"`
	Scope  string    `json:"scope"`
	Where  string    `json:"where"`
	Key    string    `json:"key,omitempty"`
	Type   string    `json:"type,omitempty"`
	Action any       `json:"action,omitempty"`
}

// writeAction appends one BlockAction/ModelAction event. scope is "model"
// or "block"; where is the block key, empty for scope "model".
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

// writeKey appends one keystroke event. where is the UI location; key is
// the human-readable key name (e.g. "enter", "esc", "down", "ctrl+c").
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

// writeMsg appends one raw tea.Msg event. tea.KeyMsg is special-cased to the
// "key" scope (see writeKey); every other message type - including the
// internal messages that drive commit/save, block open/close, delete and
// reload confirmations, doc-preset application, and validation, none of
// which dispatch a BlockAction/ModelAction - is recorded under "msg" so the
// trace has no gaps between what the user did and what got recorded.
// Messages with only unexported fields (e.g. saveResultMsg) still record
// their type name even though Action serializes to "{}": the type alone is
// enough to know the event happened.
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

// isDumpNoise reports whether msg is a high-frequency internal message that
// carries no information about what the user did, and would otherwise flood
// the trace. cursor.BlinkMsg recurs continuously while any textarea has
// focus; clearStatusMsg is yedit's own status-bar decay timer.
//
// cursor's initialBlinkMsg (fires once per textarea focus) and blinkCanceled
// (fires on every keystroke typed into a textarea - it cancels the pending
// blink) are unexported types in charmbracelet/bubbles/cursor, so they
// cannot be named in a type switch from this package; they are matched by
// their %T name instead.
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

// redactModelAction strips fields that are static schema metadata rather
// than user-generated state before an action is dumped. DrillIn.Defs is the
// worst offender: it is the fully expanded schema subtree for the field
// being drilled into, and for self-referential types (a field whose Kind
// recurses into itself, expanded SchemaRecursionDepth levels deep) that can
// balloon to megabytes per event - useless for a trace, since it is
// reconstructible from the schema and never varies with user input.
func redactModelAction(a ModelAction) ModelAction {
	if di, ok := a.(DrillIn); ok {
		return DrillIn{Key: di.Key, Kind: di.Kind, RelSegs: di.RelSegs}
	}
	return a
}

func (d *dumpWriter) path() string { return d.f.Name() }
func (d *dumpWriter) close() error { return d.f.Close() }

// wireDump composes cfg.Trace's OnAction/OnModelAction/OnMsg hooks with d,
// preserving any hooks the caller already set so Config.Trace.Dump and manual
// hooks can be used together.
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
