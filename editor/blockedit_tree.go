package editor

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/alert"
	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// fieldHasContent reports whether the field at node.YAMLPath has content in
// be.node. Reading the node rather than the text buffer keeps the check correct
// while the buffer is mid-edit or invalid.
func (be blockEditState) fieldHasContent(node fieldtree.Node) bool {
	path := node.YAMLPath
	if len(path) == 0 {
		return false
	}
	// Collection editors search from the entry value mapping, so the first path
	// segment (the entry label) is skipped.
	cur := &be.node
	start := 0
	if be.isCollectionNav() {
		entryVal := yamledit.EntryValueNode(&be.node, be.coll.isMap, be.coll.current)
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
	return child != nil && yamledit.NodeHasContent(child)
}

func (be blockEditState) updateTreePanel(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	prevSeqIdx := be.tree.NearestSeqItem()
	prevNodeIdx := be.tree.CurrentNodeIdx()

	tree, action := be.tree.Update(msg)
	be.tree = tree
	if be.tree.CurrentNodeIdx() != prevNodeIdx {
		// The hint panel now describes a different field; show it from the top.
		be.hintScroll = 0
	}

	switch action {
	case fieldtree.ActionOpenChild:
		return be.handleTreeOpenChild()
	case fieldtree.ActionToggled:
		be = be.handleTreeToggleDispatch()
	case fieldtree.ActionAddNew:
		be = be.dispatch(AddEntry{})
	case fieldtree.ActionDeleted:
		be = be.handleTreeDeleteDispatch()
	default:
		// Collection entries are shown one at a time, so moving between them flushes
		// the current buffer and loads the new entry.
		if be.isCollectionNav() {
			newSeqIdx := be.tree.NearestSeqItem()
			if newSeqIdx != prevSeqIdx {
				be = be.dispatch(NavigateEntry{Idx: newSeqIdx})
			}
		}
	}

	// Follow the selection when the cursor moved or a toggle changed the current
	// node's line. Expand/collapse leaves both unchanged, so it never jumps.
	if be.tree.CurrentNodeIdx() != prevNodeIdx || action == fieldtree.ActionToggled {
		be = be.followTreeSelection()
	}
	return be, nil
}

// handleTreeToggleDispatch confirms first, or dispatches ToggleField at once
// when NoDeleteConfirm is set or the field has no content.
func (be blockEditState) handleTreeToggleDispatch() blockEditState {
	idx := be.tree.CurrentNodeIdx()
	if idx < 0 {
		return be
	}
	node := be.tree.Nodes[idx]
	if !node.Checked && be.fieldHasContent(node) && !be.cfg.NoDeleteConfirm {
		// Revert the visual toggle while waiting for the user to confirm.
		be.tree = be.tree.WithNodeMutated(idx, func(n *fieldtree.Node) { n.Checked = true })
		capturedIdx := idx
		al := alert.NewConfirm(
			"Remove field?",
			fmt.Sprintf("Remove %q? Its content will be lost.", node.Label),
			func() tea.Msg { return pendingRemoveMsg{nodeIdx: capturedIdx} },
		)
		return be.enterConfirmAlert(al)
	}
	return be.dispatch(ToggleField{NodeIdx: idx, Checked: node.Checked})
}

// handleTreeDeleteDispatch confirms first, or dispatches DeleteEntry at once
// when NoDeleteConfirm is set.
func (be blockEditState) handleTreeDeleteDispatch() blockEditState {
	idx := be.tree.CurrentNodeIdx()
	if idx < 0 || be.tree.Nodes[idx].Kind != fieldtree.KindSeqItem {
		return be
	}
	seqIdx := be.tree.Nodes[idx].SeqIdx
	if be.cfg.NoDeleteConfirm {
		return be.dispatch(DeleteEntry{SeqIdx: seqIdx})
	}
	label := be.tree.Nodes[idx].Label
	al := alert.NewConfirm(
		"Remove entry?",
		fmt.Sprintf("Remove %q? Its content will be lost.", label),
		func() tea.Msg { return pendingEntryDeleteMsg{seqIdx: seqIdx} },
	)
	return be.enterConfirmAlert(al)
}

// handleTreeOpenChild drills into the field under the cursor, emitting an
// openChildMsg with the focus-path suffix to it. The model resolves the content
// from the canonical editRoot, so no substring is copied here.
func (be blockEditState) handleTreeOpenChild() (blockEditState, tea.Cmd) {
	idx := be.tree.CurrentNodeIdx()
	if idx < 0 {
		return be, nil
	}
	node := be.tree.Nodes[idx]

	// relSegs addresses the field relative to this editor's focus.
	var relSegs []yamledit.PathSeg
	if be.isCollectionNav() {
		// yamlPath[0] is the item's label, not a real key; the live item is
		// be.coll.current, and yamlPath[1:] are the field keys below it.
		if be.coll.isMap {
			relSegs = append(relSegs, yamledit.SegMapKey(yamledit.EntryLabel(&be.node, true, be.coll.current)))
		} else {
			relSegs = append(relSegs, yamledit.SegIdx(be.coll.current))
		}
		for _, k := range node.YAMLPath[1:] {
			relSegs = append(relSegs, yamledit.SegKey(k))
		}
	} else {
		// Struct block: node.YAMLPath is the key path from this block's mapping.
		for _, k := range node.YAMLPath {
			relSegs = append(relSegs, yamledit.SegKey(k))
		}
	}

	return be, func() tea.Msg {
		return openChildMsg{
			key:     node.Def.YAMLName,
			defs:    node.Def.Children,
			kind:    node.Def.Kind,
			relSegs: relSegs,
		}
	}
}

// applyToggle adds or removes the field at node within the canonical node, then
// re-renders the editor from it. Collections target the current entry's value
// mapping, struct blocks the block's own; the tree is derived from the same node
// either way, so it cannot disagree.
func (be blockEditState) applyToggle(ctx yamledit.ToggleCtx, node fieldtree.Node, checked bool) blockEditState {
	if be.isCollectionNav() {
		be = be.toggleEntryField(ctx, node, checked)
		// Only rebuild the buffer when the toggle succeeded: on a parse error the
		// buffer holds the invalid text, and overwriting it would mask the error.
		if be.editorErr.kind == errNone {
			be.yamlEditor.SetValue(yamledit.EntryViewYAML(&be.node, be.key, be.coll.isMap, be.coll.current))
		}
		return be
	}
	be.node = *fieldtree.ToggleNodeField(&be.node, ctx, node, checked)
	be.yamlEditor.SetValue(yamledit.NodeToContent(be.key, &be.node))
	return be
}

// toggleEntryField mutates the current collection entry's value mapping on the
// live node rather than re-parsed text. yamlPath[0] is the entry label, so the
// field path starts at [1].
func (be blockEditState) toggleEntryField(ctx yamledit.ToggleCtx, node fieldtree.Node, checked bool) blockEditState {
	if len(node.YAMLPath) < 2 {
		return be
	}
	entryNode := yamledit.EntryValueNode(&be.node, be.coll.isMap, be.coll.current)
	if entryNode == nil {
		return be
	}
	// Clone first so a mid-path yamledit.ApplyToggleAt failure cannot leave the entry
	// partially modified, mirroring fieldtree.ToggleNodeField.
	cloned := yamlnode.CloneNode(entryNode)
	fieldPath := node.YAMLPath[1:]
	if !yamledit.ApplyToggleAt(cloned, fieldPath[:len(fieldPath)-1], fieldPath[len(fieldPath)-1], checked, ctx) {
		return be
	}
	yamledit.PruneEmptyMappings(cloned)
	yamledit.ReorderNestedMappingKeys(cloned, ctx.ChildDefs)
	// Write back at the entry's position, keeping the existing key node for maps.
	var keyNode *yaml.Node
	if be.coll.isMap && 2*be.coll.current < len(be.node.Content) {
		keyNode = be.node.Content[2*be.coll.current]
	}
	yamledit.SetEntry(&be.node, be.coll.isMap, be.coll.current, keyNode, cloned)
	return be
}

// handleTreeAddNew appends a fresh entry to the collection and moves the cursor
// to it so the user can start filling in its fields immediately.
func (be blockEditState) handleTreeAddNew() blockEditState {
	be = be.saveUndo()
	be = be.flushCurrentEntry()
	be.editorErr = editorError{} // adding overrides an in-progress invalid entry
	label := be.newEntryLabel()
	be.tree = be.tree.WithNewSeqItem(be.childDefs, label)
	kn, vn, ok := yamledit.ParseEntryFromView(be.key+":\n"+be.initialEntryContent(label), be.coll.isMap)
	if !ok {
		vn = &yaml.Node{Kind: yaml.MappingNode}
		kn = &yaml.Node{Kind: yaml.ScalarNode, Value: label}
	}
	if be.coll.isMap {
		be.node.Content = append(be.node.Content, kn, vn)
	} else {
		be.node.Content = append(be.node.Content, vn)
	}
	return be.loadEntry(be.tree.NearestSeqItem())
}
