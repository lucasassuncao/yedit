package editor

import (
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// blockEditUndoSnap captures a blockEditState before a mutating operation, for
// ctrl+u and ctrl+y.
type blockEditUndoSnap struct {
	node            yaml.Node // deep copy of the canonical node at snapshot time
	currentEntryIdx int
	yamlValue       string
	preset          string
	// tree state for collection blocks, so restoring keeps the expanded view and
	// cursor position intact.
	treeNodes  []fieldtree.Node
	treeCursor int
	treeOffset int
}

const maxUndoDepth = 50

// captureSnap records the current editor state for the undo/redo stacks.
func (be blockEditState) captureSnap() blockEditUndoSnap {
	treeNodes := make([]fieldtree.Node, len(be.tree.Nodes))
	copy(treeNodes, be.tree.Nodes)
	return blockEditUndoSnap{
		node:            *yamlnode.CloneNode(&be.node),
		currentEntryIdx: be.coll.current,
		yamlValue:       be.yamlEditor.Value(),
		preset:          be.currentPreset,
		treeNodes:       treeNodes,
		treeCursor:      be.tree.Cursor,
		treeOffset:      be.tree.Offset,
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

// snapEqual compares two snapshots structurally. captureSnap builds them as deep
// copies the same way, so the comparison is exact, and reflect.DeepEqual also
// handles anchor/alias cycles.
func snapEqual(a, b blockEditUndoSnap) bool {
	return reflect.DeepEqual(a, b)
}

// saveUndo pushes the current state onto the undo stack and discards the redo
// entries, since a new mutation forks away from the undone states.
//
// An exact duplicate of the stack top is skipped: speculative checkpoints (Tab
// into the YAML panel with no edit) would otherwise pile up identical snapshots
// that make ctrl+u appear to do nothing. Unchanged state is also no fork, so the
// redo stack survives.
func (be blockEditState) saveUndo() blockEditState {
	snap := be.captureSnap()
	if n := len(be.undoStack); n > 0 && snapEqual(be.undoStack[n-1], snap) {
		return be
	}
	be.undoStack = appendSnapCapped(be.undoStack, snap)
	be.redoStack = nil
	return be
}

// restoreUndo restores the most recent snapshot that differs from the live state
// and pushes the undone state onto the redo stack. Live-equal snapshots left by
// speculative checkpoints are dropped first, so one ctrl+u always lands on a
// visible change. Sets the status message either way.
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

// restoreRedo re-applies the most recently undone change, pushing the current
// state onto the undo stack so the redo itself can be undone. Mirrors
// restoreUndo.
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

// applySnap loads snap into the live editor state. dirty is not snapshotted:
// dispatch recomputes it from the restored content, so it cannot disagree.
func (be blockEditState) applySnap(snap blockEditUndoSnap) blockEditState {
	be.currentPreset = snap.preset
	be.editorErr = editorError{}

	be.node = *yamlnode.CloneNode(&snap.node)

	if be.isCollectionNav() {
		// Clamp against the restored node's entry count so loadEntry never receives
		// an out-of-range index.
		restoredCount := yamledit.EntryCount(&be.node, be.coll.isMap)
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
			treeNodes := make([]fieldtree.Node, len(snap.treeNodes))
			copy(treeNodes, snap.treeNodes)
			be.tree.Nodes = treeNodes
			be.tree.Cursor = snap.treeCursor
			be.tree.Offset = snap.treeOffset
		} else {
			be.tree.Nodes = be.collectionTreeNodes()
			be.tree.Cursor = 0
			be.tree.Offset = 0
		}
		be = be.loadEntry(be.coll.current)
		// The node is authoritative; snap.yamlValue restores whatever in-progress,
		// possibly unparseable, text the entry held at snapshot time.
		be.yamlEditor.SetValue(snap.yamlValue)
		be.tree = be.resyncTreeFromYAML()
		be.tree = be.tree.ClampCursor()
		return be
	}
	be.yamlEditor.SetValue(snap.yamlValue)
	be.tree = fieldtree.SyncCheckedFromNode(be.tree, &be.node)
	// The cursor may now be out of bounds (it sat on a separator row that the
	// restored tree no longer has), so advance to the first selectable field
	// rather than stranding the user with no operable row.
	vis := be.tree.VisibleNodes()
	if be.tree.Cursor < 0 || be.tree.Cursor >= len(vis) {
		be.tree.Cursor = 0
		for be.tree.Cursor < len(vis) && be.tree.Nodes[vis[be.tree.Cursor]].Kind == fieldtree.KindSeparator {
			be.tree.Cursor++
		}
	}
	return be
}
