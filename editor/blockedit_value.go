package editor

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// valueChoice is a scalar whose metadata sets EnablePicker: the tree node it
// is (-1 for a tree-less block), its OneOf, and its current and default values.
type valueChoice struct {
	nodeIdx  int
	label    string
	options  []string
	current  string
	fallback string
}

// valueChoice is the scalar ctrl+o can pick a value for: the field under the
// tree's cursor, or the block itself when it is a lone scalar.
func (be blockEditState) valueChoice() (valueChoice, bool) {
	if be.cfg.Metadata == nil || be.active == blockEditPanelHint {
		return valueChoice{}, false
	}
	if be.tree.IsEmpty() {
		meta := be.fieldMeta("")
		if be.kind != schema.KindPrimitive || !meta.EnablePicker || len(meta.OneOf) == 0 {
			return valueChoice{}, false
		}
		return valueChoice{nodeIdx: -1, label: be.key, options: meta.OneOf, current: scalarValue(&be.node), fallback: meta.Default}, true
	}
	idx := be.tree.CurrentNodeIdx()
	if be.active != blockEditPanelTree || idx < 0 {
		return valueChoice{}, false
	}
	node := be.tree.Nodes[idx]
	if node.Kind != fieldtree.KindField || node.Def.Kind != schema.KindPrimitive {
		return valueChoice{}, false
	}
	meta := be.fieldMeta(be.nodeFieldPath(node))
	if !meta.EnablePicker || len(meta.OneOf) == 0 {
		return valueChoice{}, false
	}
	return valueChoice{nodeIdx: idx, label: node.Label, options: meta.OneOf, current: scalarValue(be.fieldValueNode(node)), fallback: meta.Default}, true
}

// scalarValue is n's text when it is a scalar, else "".
func scalarValue(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// openValuePicker lists the choices over the editor with the cursor on the
// current value, or the default; the answer comes back as a valuePickedMsg.
func (be blockEditState) openValuePicker() blockEditState {
	vc, ok := be.valueChoice()
	if !ok {
		return be
	}
	rows := slices.Clone(vc.options)
	cursor := slices.Index(vc.options, vc.current)
	for i, o := range vc.options {
		switch o {
		case vc.current:
			rows[i] += "  (current)"
		case vc.fallback:
			rows[i] += "  (default)"
		}
	}
	if cursor < 0 {
		cursor = slices.Index(vc.options, vc.fallback)
	}
	onPick := func(i int) tea.Cmd {
		return func() tea.Msg { return valuePickedMsg{nodeIdx: vc.nodeIdx, value: vc.options[i]} }
	}
	p := overlay.NewPick("Pick a value for "+vc.label, rows, nil, onPick, be.theme.Modal, be.theme.Legend).WithCursor(cursor)
	be.sh = be.sh.Push(p)
	be.mode = modePicking
	return be
}

// applyValue writes value into the field at nodeIdx, adding the field first
// when it is not in the YAML yet, or into the block when nodeIdx is -1.
func (be blockEditState) applyValue(nodeIdx int, value string) blockEditState {
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	if nodeIdx < 0 {
		be.node = *scalar
		be.yamlEditor.SetValue(yamledit.NodeToContent(be.key, &be.node))
		return be
	}
	if nodeIdx >= len(be.tree.Nodes) {
		return be
	}
	node := be.tree.Nodes[nodeIdx]
	be = be.applyToggle(yamledit.ToggleCtx{Snippets: be.snippetsFn(), ChildDefs: be.childDefs}, node, true)

	// Copies of the state share nodes: write into a clone, never through them.
	root := yamlnode.CloneNode(&be.node)
	path := node.YAMLPath
	target := root
	if be.isCollectionNav() {
		target = yamledit.EntryValueNode(root, be.coll.isMap, be.coll.current)
		path = path[1:]
	}
	segs := make([]yamledit.PathSeg, len(path))
	for i, k := range path {
		segs[i] = yamledit.SegKey(k)
	}
	if target == nil || len(segs) == 0 || yamledit.SetNodeAt(target, segs, scalar) != nil {
		return be
	}
	be.node = *root
	if be.isCollectionNav() {
		be.yamlEditor.SetValue(yamledit.EntryViewYAML(&be.node, be.key, be.coll.isMap, be.coll.current))
	} else {
		be.yamlEditor.SetValue(yamledit.NodeToContent(be.key, &be.node))
	}
	return be
}
