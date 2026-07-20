package editor

import "fmt"

// dispatch applies a BlockAction to be and returns the updated state.
// Every block-editor mutation passes through here; side effects
// (pruneEmptyMappings, saveUndo) are guaranteed per-case.
const maxActionLog = 512

// dispatch logs the action, applies it, and then re-derives the projected
// state at this single gateway: the tree is rebuilt from the canonical node
// and the dirty flag is recomputed against the committed baseline. No action
// can leave either disagreeing with the node, because none of them is
// responsible for keeping them in sync anymore.
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
// derivation and dirty tracking are NOT its concern - dispatch re-derives both
// after every action - so the cases only mutate the node, the buffer, and any
// structural tree rows (entry added/removed).
func (be blockEditState) applyAction(a BlockAction) blockEditState {
	switch act := a.(type) {
	case ToggleField:
		if act.NodeIdx < 0 || act.NodeIdx >= len(be.tree.nodes) {
			return be
		}
		node := be.tree.nodes[act.NodeIdx]
		be = be.saveUndo()
		ctx := toggleCtx{key: be.key, snippets: be.snippetsFn(), childDefs: be.childDefs}
		be = be.applyToggle(ctx, node, act.Checked)

	case SyncYAML:
		if act.Checkpoint {
			// Snapshot before applying so undo returns to the pre-change node.
			// Callers whose buffer has already changed by the time they dispatch
			// (e.g. a paste applied by the textarea) must push their own
			// pre-change snapshot instead of setting Checkpoint, because this
			// one would capture the post-change buffer (see updateEditing).
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
		// restoreUndo sets the status message itself: it knows whether a
		// snapshot was actually restored or the stack held only no-op states.
		be = be.restoreUndo()

	case Redo:
		be = be.restoreRedo()

	default:
		panic(fmt.Sprintf("editor: unhandled BlockAction %T", a))
	}
	return be
}

// handleNavigateEntry performs all bounds validation, undo snapshotting, and
// entry loading for a NavigateEntry action. Extracted to keep dispatch's
// cyclomatic complexity within the project limit.
func (be blockEditState) handleNavigateEntry(idx int) blockEditState {
	be.statusMsg = ""
	count := entryCount(&be.node, be.coll.isMap)
	if count == 0 || idx < 0 || idx >= count {
		// Nothing to navigate to; leave current entry unchanged.
		return be
	}
	if be.dirty {
		// Peek whether the current entry parses before committing the undo
		// snapshot: if the flush would fail we skip saveUndo so we don't
		// push a phantom step that restores the same invalid state.
		if be.flushCurrentEntry().editorErr.kind == 0 {
			be = be.saveUndo()
		}
	}
	be = be.flushAndLoadEntry(idx)
	// Surface parse errors in the status bar so they are not missed.
	if be.editorErr.kind == errParse {
		be.statusMsg = be.editorErr.message
		// The navigation was refused: move the cursor back to the entry that
		// is actually loaded, so the tree and the buffer visibly agree again
		// instead of pointing at different entries until the user notices.
		be.tree = be.tree.cursorToSeqItem(be.coll.current)
	}
	return be
}

// replayBlock replays a sequence of BlockActions from an initial state.
// Used for bug reproduction and testing.
func replayBlock(initial blockEditState, log []BlockAction) blockEditState {
	be := initial
	for _, a := range log {
		be = be.dispatch(a)
	}
	return be
}
