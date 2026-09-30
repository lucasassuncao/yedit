// Package trace records a bubbletea session to a JSONL file: every keystroke,
// message and app action with a timestamp and sequence number, so a bug report
// can be read back step by step. It knows nothing about the editor.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
)

// Writer appends events to one trace file.
type Writer struct {
	f   *os.File
	enc *json.Encoder
	seq int
}

// New creates the trace file at path, or a timestamped file in the OS temp
// dir when path is empty.
func New(path string) (*Writer, error) {
	if path == "" {
		path = filepath.Join(os.TempDir(), fmt.Sprintf("yedit-dump-%d.jsonl", time.Now().UnixNano()))
	}
	f, err := os.Create(path) // #nosec G304 -- path is supplied by the embedding application (Config.Trace.DumpPath) or generated internally
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, enc: json.NewEncoder(f)}, nil
}

// event is one line of the trace. Declaration order is the JSON key order,
// which a map would not preserve.
type event struct {
	TS     time.Time `json:"ts"`
	Seq    int       `json:"seq"`
	Scope  string    `json:"scope"`
	Where  string    `json:"where"`
	Key    string    `json:"key,omitempty"`
	Type   string    `json:"type,omitempty"`
	Action any       `json:"action,omitempty"`
}

func (w *Writer) write(ev event) {
	w.seq++
	ev.TS, ev.Seq = time.Now(), w.seq
	_ = w.enc.Encode(ev)
}

// Action appends one app action under scope (e.g. "model", "block"); where
// says which part of the app it applied to.
func (w *Writer) Action(scope, where string, action any) {
	w.write(event{Scope: scope, Where: where, Type: fmt.Sprintf("%T", action), Action: action})
}

// Key appends one keystroke, key being its readable name ("enter", "ctrl+c").
func (w *Writer) Key(where, key string) {
	w.write(event{Scope: "key", Where: where, Key: key})
}

// Msg appends one raw tea.Msg. A key message goes to the "key" scope; anything
// else lands under "msg" with its type name, even when it has no exported
// fields to serialise, so the trace has no gaps.
func (w *Writer) Msg(where string, msg tea.Msg) {
	if km, ok := msg.(tea.KeyMsg); ok {
		w.Key(where, km.String())
		return
	}
	w.write(event{Scope: "msg", Where: where, Type: fmt.Sprintf("%T", msg), Action: msg})
}

// IsNoise reports a high-frequency message that says nothing about what the
// user did and would flood the trace: cursor blinks and the status decay timer.
// The unexported ones cannot be named in a type switch, so %T matches them.
func IsNoise(msg tea.Msg) bool {
	if _, ok := msg.(cursor.BlinkMsg); ok {
		return true
	}
	switch fmt.Sprintf("%T", msg) {
	case "cursor.initialBlinkMsg", "cursor.blinkCanceled", "shell.statusExpiredMsg":
		return true
	default:
		return false
	}
}

// Path is where the trace is written.
func (w *Writer) Path() string { return w.f.Name() }

// Close flushes and closes the file.
func (w *Writer) Close() error { return w.f.Close() }
