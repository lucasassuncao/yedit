package editor

import (
	"fmt"

	"github.com/lucasassuncao/yedit/yamledit"
)

const maxActionLog = 512

// dispatch logs a BlockAction, applies it, then re-derives the projected state:
// the tree is rebuilt from the canonical node and dirty is recomputed against
// the committed baseline. Because no action keeps them in sync itself, none can
// leave them disagreeing with the node.
func (be blockEditState) dispatch(a BlockAction) blockEditState {
	if len(be.actionLog) < maxActionLog {
		log := make([]BlockAction, len(be.actionLog)+1)
		copy(log, be.actionLog)
		log[len(be.actionLog)] = a
		be.actionLog = log
	}
	be = be.applyAction(a)
	be.tree = be.resyncTreeFromYAML()
	be.dirty = be.computeDirty()
	if be.cfg.Trace.OnAction != nil {
		be.cfg.Trace.OnAction(be.key, a)
	}
	return be
}

// applyAction performs the state change for a single BlockAction. Tree
// derivation and dirty tracking belong to dispatch, so the cases only mutate the
// node, the buffer, and any structural tree rows.
func (be blockEditState) applyAction(a BlockAction) blockEditState {
	switch act := a.(type) {
	case ToggleField:
		if act.NodeIdx < 0 || act.NodeIdx >= len(be.tree.Nodes) {
			return be
		}
		node := be.tree.Nodes[act.NodeIdx]
		be = be.saveUndo()
		ctx := yamledit.ToggleCtx{Snippets: be.snippetsFn(), ChildDefs: be.childDefs}
		be = be.applyToggle(ctx, node, act.Checked)

	case SyncYAML:
		if act.Checkpoint {
			// Snapshot before applying so undo returns to the pre-change node.
			// Callers whose buffer already changed by dispatch time (a paste applied
			// by the textarea) must push their own snapshot instead, since this one
			// would capture the post-change buffer.
			be = be.saveUndo()
		}
		updated, parsed := be.syncParsedNode(act.Content)
		if !parsed {
			break
		}
		be = updated
		be.statusMsg = ""

	case AddEntry:
		be.statusMsg = ""
		be = be.handleTreeAddNew()

	case DeleteEntry:
		be.statusMsg = ""
		be = be.performEntryDelete(act.SeqIdx)

	case NavigateEntry:
		be = be.handleNavigateEntry(act.Idx)

	case ApplyPreset:
		be.statusMsg = ""
		be = be.applyPreset(act.Name, act.Content)

	case AppendPreset:
		be.statusMsg = ""
		be = be.appendPreset(act.Name, act.Content)

	case Undo:
		// restoreUndo sets the status message itself, since only it knows whether a
		// snapshot was restored or the stack held only no-op states.
		be = be.restoreUndo()

	case Redo:
		be = be.restoreRedo()

	default:
		panic(fmt.Sprintf("editor: unhandled BlockAction %T", a))
	}
	return be
}

// handleNavigateEntry does the bounds checking, undo snapshotting, and entry
// loading for a NavigateEntry action.
func (be blockEditState) handleNavigateEntry(idx int) blockEditState {
	be.statusMsg = ""
	count := yamledit.EntryCount(&be.node, be.coll.isMap)
	if count == 0 || idx < 0 || idx >= count {
		// Nothing to navigate to.
		return be
	}
	if be.dirty {
		// Peek whether the entry parses first: a failing flush would push a
		// phantom step restoring the same invalid state.
		if be.flushCurrentEntry().editorErr.kind == 0 {
			be = be.saveUndo()
		}
	}
	be = be.flushAndLoadEntry(idx)
	if be.editorErr.kind == errParse {
		be.statusMsg = be.editorErr.message
		// Navigation was refused, so move the cursor back to the entry actually
		// loaded rather than leaving the tree and buffer pointing at different ones.
		be.tree = be.tree.CursorToSeqItem(be.coll.current)
	}
	return be
}

// replayBlock replays a sequence of BlockActions from an initial state, for bug
// reproduction and testing.
func replayBlock(initial blockEditState, log []BlockAction) blockEditState {
	be := initial
	for _, a := range log {
		be = be.dispatch(a)
	}
	return be
}
