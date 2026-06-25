package editor

import (
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

// saveUndo pushes the current state onto the undo stack. Any redo entries are
// discarded - a new mutation forks away from the undone states.
func (be blockEditState) saveUndo() blockEditState {
	be.undoStack = appendSnapCapped(be.undoStack, be.captureSnap())
	be.redoStack = nil
	return be
}

// restoreUndo restores the most recent undo snapshot and pushes the undone
// state onto the redo stack. No-op when the undo stack is empty.
func (be blockEditState) restoreUndo() blockEditState {
	if len(be.undoStack) == 0 {
		return be
	}
	be.redoStack = appendSnapCapped(be.redoStack, be.captureSnap())
	snap, rest := popSnap(be.undoStack)
	be.undoStack = rest
	return be.applySnap(snap)
}

// restoreRedo re-applies the most recently undone change and pushes the
// current state onto the undo stack so the redo itself can be undone. No-op
// when the redo stack is empty.
func (be blockEditState) restoreRedo() blockEditState {
	if len(be.redoStack) == 0 {
		return be
	}
	be.undoStack = appendSnapCapped(be.undoStack, be.captureSnap())
	snap, rest := popSnap(be.redoStack)
	be.redoStack = rest
	return be.applySnap(snap)
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
		be.coll.current = snap.currentEntryIdx
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
		return be
	}
	be.yamlEditor.SetValue(snap.yamlValue)
	be.tree = syncTreeCheckedFromNode(be.tree, &be.node)
	return be
}
