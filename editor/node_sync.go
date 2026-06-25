package editor

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// This file holds the structural (node-based) counterparts of the text-parsing
// sync helpers. They read/derive tree state directly from a *yaml.Node - the
// single source of truth - instead of re-parsing the YAML text on every edit.
// See the "single source of truth" design: the tree is a pure projection of the
// node, so the two can never disagree even while the text buffer is mid-edit.

// deriveChecked recomputes the checked flag of every leaf/openable field node
// from valueNode (the block's value mapping). It is the structural replacement
// for syncTreeCheckedStates / syncCurrentEntry / syncMapEntryChecked.
//
// skipFirstSeg drops yamlPath[0] for nodes below depth 0 - used for collection
// entries, where path[0] is the entry label, not a real mapping key. The
// returned slice is a copy; the input is left untouched.
func deriveChecked(valueNode *yaml.Node, nodes []treeNode, skipFirstSeg bool) []treeNode {
	out := make([]treeNode, len(nodes))
	copy(out, nodes)
	for i, n := range out {
		// Leaves and inline-struct parents track key presence; openable fields
		// (nested collections) track non-empty content.
		if n.kind != treeNodeField {
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
			cur = yamlnode.ChildByKey(cur, path[j])
		}
		if cur == nil {
			out[i].checked = false
			continue
		}
		child := yamlnode.ChildByKey(cur, path[len(path)-1])
		if n.openable || !n.isLeaf {
			// Openable fields and inline struct parents require real content, not
			// just key presence, so an empty mapping {} counts as unchecked.
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
// block's value mapping). It clones valueNode before any mutation and returns
// the (possibly new) node; the caller must assign the result back. Returns
// valueNode unchanged when the toggle produces no structural change.
func toggleNodeField(valueNode *yaml.Node, ctx toggleCtx, node treeNode, checked bool) *yaml.Node {
	cloned := yamlnode.CloneNode(valueNode)
	if cloned.Kind != yaml.MappingNode {
		cloned.Kind = yaml.MappingNode
		cloned.Tag = ""
		cloned.Value = ""
		cloned.Content = nil
	}
	path := node.yamlPath
	asStruct := node.depth == 0 && node.def.Kind == schema.KindObject
	if !applyToggleAt(cloned, path[:len(path)-1], path[len(path)-1], checked, ctx, asStruct) {
		return valueNode
	}
	pruneEmptyMappings(cloned)
	reorderNestedMappingKeys(cloned, ctx.childDefs)
	return cloned
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

// blockValueNode parses content of the form "<key>:\n  ..." and returns the
// value node mapped to key - the canonical node for a block editor. When the
// content is empty or unparseable it returns an empty mapping so a fresh block
// always has a writable node. It reuses valueNodeOfSnippet for the happy path.
func blockValueNode(content string) *yaml.Node {
	if v := valueNodeOfSnippet(content); v != nil {
		return v
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}

// blockValueNodeOrNil is like blockValueNode but returns nil when content is
// non-empty and fails to parse, instead of silently falling back to an empty
// mapping. Used by callers that need to distinguish "empty block" from "corrupt
// content" so they can surface an error rather than masking it.
func blockValueNodeOrNil(content string) *yaml.Node {
	if strings.TrimSpace(content) == "" {
		return &yaml.Node{Kind: yaml.MappingNode}
	}
	return valueNodeOfSnippet(content)
}
