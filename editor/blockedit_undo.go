package editor

import (
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/internal/yamlnode"
)

// blockEditUndoSnap captures the state of a blockEditState before a
// mutating operation so it can be restored by ctrl+u.
type blockEditUndoSnap struct {
	node            *yaml.Node // deep copy of the canonical node at snapshot time
	currentEntryIdx int
	yamlValue       string
	dirty           bool
	preset          string
	// tree state for collection blocks — preserved so restoreUndo keeps
	// the expanded/collapsed view and cursor position intact.
	treeNodes  []treeNode
	treeCursor int
	treeOffset int
}

const maxUndoDepth = 50

func (be blockEditState) saveUndo() blockEditState {
	treeNodes := make([]treeNode, len(be.tree.nodes))
	copy(treeNodes, be.tree.nodes)
	snap := &blockEditUndoSnap{
		node:            yamlnode.CloneNode(be.node),
		currentEntryIdx: be.coll.current,
		yamlValue:       be.yamlEditor.Value(),
		dirty:           be.dirty,
		preset:          be.currentPreset,
		treeNodes:       treeNodes,
		treeCursor:      be.tree.cursor,
		treeOffset:      be.tree.offset,
	}
	be.undoStack = append(be.undoStack, snap)
	if len(be.undoStack) > maxUndoDepth {
		drop := len(be.undoStack) - maxUndoDepth
		for i := range drop {
			be.undoStack[i] = nil
		}
		be.undoStack = be.undoStack[drop:]
	}
	return be
}

func (be blockEditState) restoreUndo() blockEditState {
	if len(be.undoStack) == 0 {
		return be
	}
	last := len(be.undoStack) - 1
	snap := be.undoStack[last]
	be.undoStack[last] = nil
	be.undoStack = be.undoStack[:last]
	be.currentPreset = snap.preset
	be.dirty = snap.dirty
	be.errMsg = ""

	be.node = yamlnode.CloneNode(snap.node)

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
	be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	return be
}
