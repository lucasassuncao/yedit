package editor

import "gopkg.in/yaml.v3"

// This file holds the structural (node-based) counterparts of the text-parsing
// sync helpers. They read/derive tree state directly from a *yaml.Node — the
// single source of truth — instead of re-parsing the YAML text on every edit.
// See the "single source of truth" design: the tree is a pure projection of the
// node, so the two can never disagree even while the text buffer is mid-edit.

// deriveChecked recomputes the checked flag of every leaf/openable field node
// from valueNode (the block's value mapping). It is the structural replacement
// for syncTreeCheckedStates / syncCurrentEntry / syncMapEntryChecked.
//
// skipFirstSeg drops yamlPath[0] for nodes below depth 0 — used for collection
// entries, where path[0] is the entry label, not a real mapping key. The
// returned slice is a copy; the input is left untouched.
func deriveChecked(valueNode *yaml.Node, nodes []treeNode, skipFirstSeg bool) []treeNode {
	out := make([]treeNode, len(nodes))
	copy(out, nodes)
	for i, n := range out {
		// Leaves track key presence; openable fields (nested collections) track
		// non-empty content. Inline-expandable structs derive their state from
		// descendants (hasCheckedDescendant), so they are skipped here.
		if n.kind != treeNodeField || (!n.isLeaf && !n.openable) {
			continue
		}
		path := n.yamlPath
		start := 0
		if skipFirstSeg && n.depth > 0 {
			start = 1
		}
		if len(path) <= start {
			out[i].checked = false
			continue
		}
		cur := valueNode
		for j := start; j < len(path)-1 && cur != nil; j++ {
			cur = childByKey(cur, path[j])
		}
		if cur == nil {
			out[i].checked = false
			continue
		}
		child := childByKey(cur, path[len(path)-1])
		if n.openable {
			out[i].checked = child != nil && nodeHasContent(child)
		} else {
			out[i].checked = child != nil
		}
	}
	return out
}

// syncTreeCheckedFromNode re-derives checked states for all field nodes from
// valueNode (the block's canonical value mapping), then re-applies ADDED/
// AVAILABLE sectioning for struct trees, restoring the cursor. It is the
// node-based replacement for syncTreeCheckedFromYAML.
func syncTreeCheckedFromNode(tm treeModel, valueNode *yaml.Node) treeModel {
	var selectedPath []string
	if ni := tm.currentNodeIdx(); ni >= 0 && tm.nodes[ni].kind == treeNodeField {
		selectedPath = tm.nodes[ni].yamlPath
	}

	tm.nodes = deriveChecked(valueNode, tm.nodes, false)

	if !tm.isSeq {
		tm.nodes = applySections(tm.nodes)
		tm = tm.restoreCursorToPath(selectedPath)
	}
	return tm
}

// toggleNodeField adds or removes a single leaf field within valueNode (the
// block's value mapping), mutating it in place. It is the structural
// replacement for applyTreeToggle: same logic, no string round-trip. A
// null/scalar value node is coerced to an empty mapping first so fields can be
// added to a block opened with no content yet.
func toggleNodeField(valueNode *yaml.Node, ctx toggleCtx, node treeNode, checked bool) {
	if valueNode.Kind != yaml.MappingNode {
		valueNode.Kind = yaml.MappingNode
		valueNode.Tag = ""
		valueNode.Value = ""
		valueNode.Content = nil
	}
	path := node.yamlPath
	if !applyToggleAt(valueNode, path[:len(path)-1], path[len(path)-1], checked, ctx, node.depth == 0) {
		return
	}
	pruneEmptyMappings(valueNode)
	reorderNestedMappingKeys(valueNode, ctx.childDefs)
}

// nodeHasContent reports whether a value node carries real content. It is the
// node-level analogue of nonEmptyYAMLValue: a null scalar, empty string, or
// empty list/map counts as empty.
func nodeHasContent(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Tag != "!!null" && n.Value != ""
	case yaml.SequenceNode, yaml.MappingNode:
		return len(n.Content) > 0
	case yaml.AliasNode:
		return true
	default:
		return false
	}
}

// cloneNode returns a deep copy of n so a snapshot can be mutated independently
// of the live tree (used by saveUndo). Returns nil for a nil input.
func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	cp := *n
	if n.Content != nil {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			cp.Content[i] = cloneNode(c)
		}
	}
	if n.Alias != nil {
		cp.Alias = cloneNode(n.Alias)
	}
	return &cp
}

// blockValueNode parses content of the form "<key>:\n  ..." and returns the
// value node mapped to key — the canonical node for a block editor. When the
// content is empty or unparseable it returns an empty mapping so a fresh block
// always has a writable node. It reuses valueNodeOfSnippet for the happy path.
func blockValueNode(content string) *yaml.Node {
	if v := valueNodeOfSnippet(content); v != nil {
		return v
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}
