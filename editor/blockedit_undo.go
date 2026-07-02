package editor

import (
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/yamlnode"
)

// blockEditUndoSnap captures the state of a blockEditState before a
// mutating operation so it can be restored by ctrl+u (and re-applied by
// ctrl+y after an undo).
type blockEditUndoSnap struct {
	node            yaml.Node // deep copy of the canonical node at snapshot time
	currentEntryIdx int
	yamlValue       string
	dirty           bool
	preset          string
	// tree state for collection blocks - preserved so restoring keeps
	// the expanded/collapsed view and cursor position intact.
	treeNodes  []treeNode
	treeCursor int
	treeOffset int
}

const maxUndoDepth = 50

// captureSnap records the current editor state as a snapshot for the
// undo/redo stacks.
func (be blockEditState) captureSnap() blockEditUndoSnap {
	treeNodes := make([]treeNode, len(be.tree.nodes))
	copy(treeNodes, be.tree.nodes)
	return blockEditUndoSnap{
		node:            *yamlnode.CloneNode(&be.node),
		currentEntryIdx: be.coll.current,
		yamlValue:       be.yamlEditor.Value(),
		dirty:           be.dirty,
		preset:          be.currentPreset,
		treeNodes:       treeNodes,
		treeCursor:      be.tree.cursor,
		treeOffset:      be.tree.offset,
	}
}

// appendSnapCapped appends snap to stack, dropping the oldest entries beyond
// maxUndoDepth.
func appendSnapCapped(stack []blockEditUndoSnap, snap blockEditUndoSnap) []blockEditUndoSnap {
	stack = append(stack, snap)
	if len(stack) > maxUndoDepth {
		stack = stack[len(stack)-maxUndoDepth:]
	}
	return stack
}

// snapEqual reports whether two snapshots capture the same editor state.
// Snapshots are deep copies built the same way by captureSnap, so a structural
// comparison is exact; reflect.DeepEqual also handles anchor/alias cycles.
func snapEqual(a, b blockEditUndoSnap) bool {
	return reflect.DeepEqual(a, b)
}

// saveUndo pushes the current state onto the undo stack. Any redo entries are
// discarded - a new mutation forks away from the undone states.
//
// An exact duplicate of the stack top is skipped: speculative checkpoints
// (e.g. Tab into the YAML panel with no edit afterwards) would otherwise pile
// up identical snapshots that make ctrl+u appear to do nothing. When the state
// is unchanged since the last push there is no fork to discard either, so the
// redo stack is left alone.
func (be blockEditState) saveUndo() blockEditState {
	snap := be.captureSnap()
	if n := len(be.undoStack); n > 0 && snapEqual(be.undoStack[n-1], snap) {
		return be
	}
	be.undoStack = appendSnapCapped(be.undoStack, snap)
	be.redoStack = nil
	return be
}

// restoreUndo restores the most recent undo snapshot that differs from the
// live state and pushes the undone state onto the redo stack. Snapshots equal
// to the live state (left by speculative checkpoints) are dropped first, so a
// single ctrl+u always lands on a visible change. Sets the matching status
// message; no-op apart from it when there is nothing to restore.
func (be blockEditState) restoreUndo() blockEditState {
	live := be.captureSnap()
	for len(be.undoStack) > 0 && snapEqual(be.undoStack[len(be.undoStack)-1], live) {
		be.undoStack = be.undoStack[:len(be.undoStack)-1]
	}
	if len(be.undoStack) == 0 {
		be.statusMsg = "Nothing to undo."
		return be
	}
	be.redoStack = appendSnapCapped(be.redoStack, live)
	snap, rest := popSnap(be.undoStack)
	be.undoStack = rest
	be = be.applySnap(snap)
	be.statusMsg = "Undone."
	return be
}

// restoreRedo re-applies the most recently undone change and pushes the
// current state onto the undo stack so the redo itself can be undone. Mirrors
// restoreUndo: live-equal snapshots are dropped and the status message is set
// here; no-op apart from it when there is nothing to restore.
func (be blockEditState) restoreRedo() blockEditState {
	live := be.captureSnap()
	for len(be.redoStack) > 0 && snapEqual(be.redoStack[len(be.redoStack)-1], live) {
		be.redoStack = be.redoStack[:len(be.redoStack)-1]
	}
	if len(be.redoStack) == 0 {
		be.statusMsg = "Nothing to redo."
		return be
	}
	be.undoStack = appendSnapCapped(be.undoStack, live)
	snap, rest := popSnap(be.redoStack)
	be.redoStack = rest
	be = be.applySnap(snap)
	be.statusMsg = "Redone."
	return be
}

// popSnap removes and returns the top snapshot of stack.
func popSnap(stack []blockEditUndoSnap) (blockEditUndoSnap, []blockEditUndoSnap) {
	last := len(stack) - 1
	snap := stack[last]
	return snap, stack[:last]
}

// applySnap loads snap into the live editor state.
func (be blockEditState) applySnap(snap blockEditUndoSnap) blockEditState {
	be.currentPreset = snap.preset
	be.dirty = snap.dirty
	be.editorErr = editorError{}

	be.node = *yamlnode.CloneNode(&snap.node)

	if be.isCollectionNav() {
		// Clamp the restored entry index against the actual entry count in the
		// restored node to prevent loadEntry from receiving an out-of-range index.
		restoredCount := entryCount(&be.node, be.coll.isMap)
		idx := snap.currentEntryIdx
		switch {
		case restoredCount == 0:
			idx = -1
		case idx >= restoredCount:
			idx = restoredCount - 1
		case idx < 0:
			idx = 0
		}
		be.coll.current = idx
		if len(snap.treeNodes) > 0 {
			treeNodes := make([]treeNode, len(snap.treeNodes))
			copy(treeNodes, snap.treeNodes)
			be.tree.nodes = treeNodes
			be.tree.cursor = snap.treeCursor
			be.tree.offset = snap.treeOffset
		} else {
			be.tree.nodes = be.collectionTreeNodes()
			be.tree.cursor = 0
			be.tree.offset = 0
		}
		be = be.loadEntry(be.coll.current)
		// The node is authoritative; snap.yamlValue restores any in-progress (even
		// unparseable) text the user had typed into the entry at snapshot time.
		be.yamlEditor.SetValue(snap.yamlValue)
		be.tree = be.resyncTreeFromYAML()
		be.tree = be.tree.clampCursor()
		return be
	}
	be.yamlEditor.SetValue(snap.yamlValue)
	be.tree = syncTreeCheckedFromNode(be.tree, &be.node)
	// If syncTreeCheckedFromNode left the cursor out of bounds (e.g. the pre-
	// undo cursor was on an AVAILABLE/separator row that no longer exists in
	// the restored tree), advance to the first selectable field so the user is
	// not silently stranded with no operable row.
	vis := be.tree.visibleNodes()
	if be.tree.cursor < 0 || be.tree.cursor >= len(vis) {
		be.tree.cursor = 0
		for be.tree.cursor < len(vis) && be.tree.nodes[vis[be.tree.cursor]].kind == treeNodeSeparator {
			be.tree.cursor++
		}
	}
	return be
}
