package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/internal/alert"
)

// fieldHasContent reports whether the field at node.yamlPath has a non-empty
// value in the current YAML editor content. For structured sequences the
// editor shows a single item under the block key, so doc[key] is a []any with
// one element and node.yamlPath[0] is the seq-item label (skipped).
func (be blockEditState) fieldHasContent(node treeNode) bool {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(be.yamlEditor.Value()), &doc); err != nil {
		return false
	}
	path := node.yamlPath
	if len(path) == 0 {
		return false
	}
	var sub map[string]any
	startIdx := 0
	if items, ok := doc[be.key].([]any); ok {
		if len(items) == 0 {
			return false
		}
		sub, _ = items[0].(map[string]any)
		startIdx = 1 // skip seq item label
	} else {
		sub, _ = doc[be.key].(map[string]any)
	}
	if sub == nil || len(path) <= startIdx {
		return false
	}
	cur := sub
	for i := startIdx; i < len(path)-1; i++ {
		cur, _ = cur[path[i]].(map[string]any)
		if cur == nil {
			return false
		}
	}
	val, exists := cur[path[len(path)-1]]
	if !exists || val == nil {
		return false
	}
	switch v := val.(type) {
	case string:
		return v != ""
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	default:
		return true
	}
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
		relSegs = append(relSegs, segIdx(be.coll.current))
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
func (be *blockEditState) applyToggle(ctx toggleCtx, node treeNode, checked bool) {
	if be.isCollectionNav() {
		be.toggleEntryField(ctx, node, checked)
		be.yamlEditor.SetValue(entryViewYAML(&be.node, be.key, be.coll.isMap, be.coll.current))
		return
	}
	be.node = *toggleNodeField(&be.node, ctx, node, checked)
	be.yamlEditor.SetValue(nodeToContent(be.key, &be.node))
}

// toggleEntryField mutates the current collection entry's value mapping. It
// mirrors applyToggleToEntry but operates on the live node instead of re-parsed
// text: yamlPath[0] is the entry label (skipped), the field path starts at [1].
func (be *blockEditState) toggleEntryField(ctx toggleCtx, node treeNode, checked bool) {
	if len(node.yamlPath) < 2 {
		return
	}
	entryNode := entryValueNode(&be.node, be.coll.isMap, be.coll.current)
	if entryNode == nil {
		return
	}
	fieldPath := node.yamlPath[1:]
	if !applyToggleAt(entryNode, fieldPath[:len(fieldPath)-1], fieldPath[len(fieldPath)-1], checked, ctx, false) {
		return
	}
	pruneEmptyMappings(entryNode)
	reorderNestedMappingKeys(entryNode, ctx.childDefs)
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
