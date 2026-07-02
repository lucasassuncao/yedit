package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/alert"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// fieldHasContent reports whether the field at node.yamlPath has non-empty
// content in the canonical node (be.node). Using the node rather than parsing
// the text buffer means the check is correct even when the buffer is mid-edit
// or temporarily invalid.
func (be blockEditState) fieldHasContent(node treeNode) bool {
	path := node.yamlPath
	if len(path) == 0 {
		return false
	}
	// For collection editors the entry value mapping is the search root;
	// skip the first path segment (entry label).
	cur := &be.node
	start := 0
	if be.isCollectionNav() {
		entryVal := entryValueNode(&be.node, be.coll.isMap, be.coll.current)
		if entryVal == nil {
			return false
		}
		cur = entryVal
		start = 1
	}
	for j := start; j < len(path)-1; j++ {
		cur = yamlnode.ChildByKey(cur, path[j])
		if cur == nil {
			return false
		}
	}
	child := yamlnode.ChildByKey(cur, path[len(path)-1])
	return child != nil && nodeHasContent(child)
}

func (be blockEditState) updateTreePanel(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	prevSeqIdx := be.tree.NearestSeqItem()

	tree, action := be.tree.Update(msg)
	be.tree = tree

	switch action {
	case treeOpenChild:
		return be.handleTreeOpenChild()
	case treeToggled:
		be = be.handleTreeToggleDispatch()
		return be, nil
	case treeAddNew:
		be = be.dispatch(AddEntry{})
		return be, nil
	case treeDeleted:
		be = be.handleTreeDeleteDispatch()
		return be, nil
	}

	// Collection entries are shown one at a time; moving to a different entry
	// requires flushing the current buffer and loading the new entry.
	if be.isCollectionNav() && (action == treeNoAction || action == treeExpanded || action == treeCollapsed) {
		newSeqIdx := be.tree.NearestSeqItem()
		if newSeqIdx != prevSeqIdx {
			be = be.dispatch(NavigateEntry{Idx: newSeqIdx})
		}
	}

	return be, nil
}

// handleTreeToggleDispatch either shows a confirmation dialog or dispatches
// ToggleField immediately (when NoDeleteConfirm or the field has no content).
func (be blockEditState) handleTreeToggleDispatch() blockEditState {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be
	}
	node := be.tree.nodes[idx]
	if !node.checked && be.fieldHasContent(node) && !be.cfg.NoDeleteConfirm {
		// Revert the visual toggle while waiting for the user to confirm.
		be.tree = be.tree.withNodeMutated(idx, func(n *treeNode) { n.checked = true })
		capturedIdx := idx
		al := alert.NewConfirm(
			"Remove field?",
			fmt.Sprintf("Remove %q? Its content will be lost.", node.label),
			func() tea.Msg { return pendingRemoveMsg{nodeIdx: capturedIdx} },
		)
		be.confirmAlert = al
		be.confirmAlertVisible = true
		be.mode = modeConfirming
		return be
	}
	return be.dispatch(ToggleField{NodeIdx: idx, Checked: node.checked})
}

// handleTreeDeleteDispatch either shows a confirmation dialog or dispatches
// DeleteEntry immediately (when NoDeleteConfirm).
func (be blockEditState) handleTreeDeleteDispatch() blockEditState {
	idx := be.tree.currentNodeIdx()
	if idx < 0 || be.tree.nodes[idx].kind != treeNodeSeqItem {
		return be
	}
	seqIdx := be.tree.nodes[idx].seqIdx
	if be.cfg.NoDeleteConfirm {
		return be.dispatch(DeleteEntry{SeqIdx: seqIdx})
	}
	label := be.tree.nodes[idx].label
	al := alert.NewConfirm(
		"Remove entry?",
		fmt.Sprintf("Remove %q? Its content will be lost.", label),
		func() tea.Msg { return pendingEntryDeleteMsg{seqIdx: seqIdx} },
	)
	be.confirmAlert = al
	be.confirmAlertVisible = true
	be.mode = modeConfirming
	return be
}

// handleTreeOpenChild drills into the openable field under the cursor by
// emitting an openChildMsg carrying the focus-path suffix from this editor to
// the drilled-into node. The model resolves the actual content from the
// canonical editRoot, so no substring is copied here.
func (be blockEditState) handleTreeOpenChild() (blockEditState, tea.Cmd) {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be, nil
	}
	node := be.tree.nodes[idx]

	// relSegs addresses the field relative to this editor's focus.
	var relSegs []pathSeg
	if be.isCollectionNav() {
		// node.yamlPath[0] is the current item's label (not a real key); the live
		// item is be.coll.current. node.yamlPath[1:] are the field keys below it.
		if be.coll.isMap {
			relSegs = append(relSegs, segMapKey(entryLabel(&be.node, true, be.coll.current)))
		} else {
			relSegs = append(relSegs, segIdx(be.coll.current))
		}
		for _, k := range node.yamlPath[1:] {
			relSegs = append(relSegs, segKey(k))
		}
	} else {
		// Struct block: node.yamlPath is the key path from this block's mapping.
		for _, k := range node.yamlPath {
			relSegs = append(relSegs, segKey(k))
		}
	}

	return be, func() tea.Msg {
		return openChildMsg{
			key:     node.def.YAMLName,
			defs:    node.def.Children,
			kind:    node.def.Kind,
			relSegs: relSegs,
		}
	}
}

// applyToggle adds or removes the field at node within the canonical node, then
// re-renders the editor from it. For collections it targets the current entry's
// value mapping; for struct blocks the block's own mapping. Either way the tree
// (derived from the same node) stays in agreement.
func (be blockEditState) applyToggle(ctx toggleCtx, node treeNode, checked bool) blockEditState {
	if be.isCollectionNav() {
		be = be.toggleEntryField(ctx, node, checked)
		// Only rebuild the YAML editor from the canonical node when the toggle
		// succeeded (no parse error). If there IS a parse error the buffer
		// already contains invalid text; overwriting it with the canonical
		// content would mask the error and confuse the user.
		if be.editorErr.kind == errNone {
			be.yamlEditor.SetValue(entryViewYAML(&be.node, be.key, be.coll.isMap, be.coll.current))
		}
		return be
	}
	be.node = *toggleNodeField(&be.node, ctx, node, checked)
	be.yamlEditor.SetValue(nodeToContent(be.key, &be.node))
	return be
}

// toggleEntryField mutates the current collection entry's value mapping. It
// mirrors applyToggleToEntry but operates on the live node instead of re-parsed
// text: yamlPath[0] is the entry label (skipped), the field path starts at [1].
func (be blockEditState) toggleEntryField(ctx toggleCtx, node treeNode, checked bool) blockEditState {
	if len(node.yamlPath) < 2 {
		return be
	}
	entryNode := entryValueNode(&be.node, be.coll.isMap, be.coll.current)
	if entryNode == nil {
		return be
	}
	// Clone before any mutation so a failed applyToggleAt mid-path does not
	// leave the entry in a partially-modified state (mirrors toggleNodeField).
	cloned := yamlnode.CloneNode(entryNode)
	fieldPath := node.yamlPath[1:]
	// asStruct=true for KindObject fields at depth 1 so their snippet is nested
	// correctly under the field name (same logic as toggleNodeField for structs).
	asStruct := node.def.Kind == schema.KindObject && len(fieldPath) == 1
	if !applyToggleAt(cloned, fieldPath[:len(fieldPath)-1], fieldPath[len(fieldPath)-1], checked, ctx, asStruct) {
		return be
	}
	pruneEmptyMappings(cloned)
	reorderNestedMappingKeys(cloned, ctx.childDefs)
	// Write the cloned node back into be.node at the entry's position.
	if be.coll.isMap {
		idx := be.coll.current
		if 2*idx+1 < len(be.node.Content) {
			be.node.Content[2*idx+1] = cloned
		}
	} else {
		idx := be.coll.current
		if idx >= 0 && idx < len(be.node.Content) {
			be.node.Content[idx] = cloned
		}
	}
	return be
}

// handleTreeAddNew appends a fresh entry to the collection and moves the cursor
// to it so the user can start filling in its fields immediately.
func (be blockEditState) handleTreeAddNew() blockEditState {
	be = be.saveUndo()
	be.dirty = true
	be = be.flushCurrentEntry()
	be.editorErr = editorError{} // adding overrides an in-progress invalid entry; don't block on it
	label := be.newEntryLabel()
	be.tree = be.tree.WithNewSeqItem(be.childDefs, label)
	// Build the new entry node from the schema template and append it.
	kn, vn, ok := parseEntryFromView(be.key+":\n"+be.initialEntryContent(label), be.coll.isMap)
	if !ok {
		vn = &yaml.Node{Kind: yaml.MappingNode}
		kn = &yaml.Node{Kind: yaml.ScalarNode, Value: label}
	}
	if be.coll.isMap {
		be.node.Content = append(be.node.Content, kn, vn)
	} else {
		be.node.Content = append(be.node.Content, vn)
	}
	be = be.loadEntry(be.tree.NearestSeqItem())
	be.tree = be.resyncTreeFromYAML()
	return be
}

// performEntryDelete removes collection entry seqIdx from both the tree and the
// buffer, saving undo first. saveUndo runs before the tree is mutated, so the
// snapshot captures the pre-deletion tree directly and ctrl+u restores the entry.
