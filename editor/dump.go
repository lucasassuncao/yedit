package editor

import (
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/trace"
)

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

// wireDump composes cfg.Trace's hooks with d, preserving any the caller already
// set so Config.Trace.Dump and manual hooks work together.
func wireDump(cfg *Config, d *trace.Writer) {
	prevAction := cfg.Trace.OnAction
	cfg.Trace.OnAction = func(blockKey string, a BlockAction) {
		d.Action("block", blockKey, a)
		if prevAction != nil {
			prevAction(blockKey, a)
		}
	}

	prevModelAction := cfg.Trace.OnModelAction
	cfg.Trace.OnModelAction = func(a ModelAction) {
		d.Action("model", "", redactModelAction(a))
		if prevModelAction != nil {
			prevModelAction(a)
		}
	}

	prevMsg := cfg.Trace.OnMsg
	cfg.Trace.OnMsg = func(where string, msg tea.Msg) {
		if !trace.IsNoise(msg) {
			d.Msg(where, msg)
		}
		if prevMsg != nil {
			prevMsg(where, msg)
		}
	}
}
