package editor

// dispatch applies a BlockAction to be and returns the updated state.
// Every block-editor mutation passes through here; side effects
// (pruneEmptyMappings, saveUndo) are guaranteed per-case.
func (be blockEditState) dispatch(a BlockAction) blockEditState {
	log := make([]BlockAction, len(be.actionLog)+1)
	copy(log, be.actionLog)
	log[len(be.actionLog)] = a
	be.actionLog = log
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
		be = be.syncParsedNode(act.Content)
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
		be = be.flushAndLoadEntry(act.Idx)

	case ApplyPreset:
		be.statusMsg = ""
		be = be.applyPreset(act.Name, act.Content)

	case Undo:
		be = be.restoreUndo()
		be.statusMsg = "Undone."

	case Redo:
		be = be.restoreRedo()
		be.statusMsg = "Redone."
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
