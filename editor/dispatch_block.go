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
		be.applyToggle(ctx, node, act.Checked)
		be.tree = be.resyncTreeFromYAML()

	case SyncYAML:
		updated, parsed := be.syncParsedNode(act.Content)
		if !parsed {
			break
		}
		if act.Checkpoint {
			be = be.saveUndo()
			updated, _ = be.syncParsedNode(act.Content)
		}
		be = updated
		be.dirty = true
		be.statusMsg = ""

	case AddEntry:
		be.statusMsg = ""
		be = be.handleTreeAddNew()

	case DeleteEntry:
		be.statusMsg = ""
		be = be.performEntryDelete(act.SeqIdx)

	case NavigateEntry:
		be.statusMsg = ""
		if be.dirty {
			// Peek whether the current entry parses before committing the undo
			// snapshot: if the flush would fail we skip saveUndo so we don't
			// push a phantom step that restores the same invalid state.
			if be.flushCurrentEntry().editorErr.kind == 0 {
				be = be.saveUndo()
			}
		}
		be = be.flushAndLoadEntry(act.Idx)

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

// replayBlock replays a sequence of BlockActions from an initial state.
// Used for bug reproduction and testing.
func replayBlock(initial blockEditState, log []BlockAction) blockEditState {
	be := initial
	for _, a := range log {
		be = be.dispatch(a)
	}
	return be
}
