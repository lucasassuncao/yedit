package editor

import "fmt"

// dispatch applies a BlockAction to be and returns the updated state.
// Every block-editor mutation passes through here; side effects
// (pruneEmptyMappings, saveUndo) are guaranteed per-case.
const maxActionLog = 512

func (be blockEditState) dispatch(a BlockAction) blockEditState {
	if len(be.actionLog) < maxActionLog {
		log := make([]BlockAction, len(be.actionLog)+1)
		copy(log, be.actionLog)
		log[len(be.actionLog)] = a
		be.actionLog = log
	}
	switch act := a.(type) {
	case ToggleField:
		if act.NodeIdx < 0 || act.NodeIdx >= len(be.tree.nodes) {
			return be
		}
		node := be.tree.nodes[act.NodeIdx]
		be = be.saveUndo()
		be.dirty = true
		ctx := toggleCtx{key: be.key, snippets: be.snippetsFn(), childDefs: be.childDefs}
		be = be.applyToggle(ctx, node, act.Checked)
		be.tree = be.resyncTreeFromYAML()
		if !be.isCollectionNav() && be.committedYAML != "" && be.yamlEditor.Value() == be.committedYAML {
			be.dirty = false
		}

	case SyncYAML:
		if act.Checkpoint {
			// Snapshot BEFORE applying the change so undo returns to the
			// pre-paste/pre-batch-edit state, not the post-change state.
			be = be.saveUndo()
		}
		updated, parsed := be.syncParsedNode(act.Content)
		if !parsed {
			break
		}
		be = updated
		be.dirty = true
		if !be.isCollectionNav() && be.committedYAML != "" && be.yamlEditor.Value() == be.committedYAML {
			be.dirty = false
		}
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
		be = be.restoreUndo()
		be.statusMsg = "Undone."

	case Redo:
		be = be.restoreRedo()
		be.statusMsg = "Redone."

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
