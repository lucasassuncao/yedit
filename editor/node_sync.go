package editor

import (
	"sort"
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
			// A present-but-empty leaf (null/""/[]/{}) is scaffolding the user has
			// not filled in; pruneEmptyContent strips it at save, so flag it here
			// to render it as a draft rather than a committed field.
			out[i].emptyValue = child != nil && !nodeHasContent(child)
		}
	}
	return out
}

// syncTreeCheckedFromNode re-derives checked states for all field nodes from
// valueNode (the block's canonical value mapping), then re-applies ADDED/
// AVAILABLE/UNKNOWN sectioning for struct trees, restoring the cursor. It is the
// node-based replacement for syncTreeCheckedFromYAML.
func syncTreeCheckedFromNode(tm treeModel, valueNode *yaml.Node) treeModel {
	var selectedPath []string
	if ni := tm.currentNodeIdx(); ni >= 0 && tm.nodes[ni].kind == treeNodeField {
		selectedPath = tm.nodes[ni].yamlPath
	}

	tm.nodes = deriveChecked(valueNode, tm.nodes, false)

	if !tm.isSeq {
		tm.nodes = applySections(tm.nodes, collectUnknownNodes(valueNode, tm.defs))
		tm.nodes = injectNestedUnknowns(tm.nodes, valueNode, tm.defs)
		tm = tm.restoreCursorToPath(selectedPath)
	}
	return tm
}

// injectNestedUnknowns scans the flat node list for inline struct parents
// (treeNodeField, !isLeaf, !openable) at any depth and inserts treeNodeUnknown
// children after each parent's descendant run when the parent's value mapping
// contains keys not declared in its schema. applySections strips all
// treeNodeUnknown nodes before re-calling this, keeping it idempotent.
func injectNestedUnknowns(nodes []treeNode, valueNode *yaml.Node, defs []schema.FieldDef) []treeNode {
	if valueNode == nil || valueNode.Kind != yaml.MappingNode || len(defs) == 0 {
		return nodes
	}
	// insertion queues unknown rows to be emitted right after nodes[after],
	// i.e. past the parent's contiguous run of descendants.
	type insertion struct {
		after int
		rows  []treeNode
	}
	var insertions []insertion
	for i, n := range nodes {
		if n.kind != treeNodeField || n.isLeaf || n.openable || len(n.yamlPath) == 0 {
			continue
		}
		d, ok := defAtPath(defs, n.yamlPath)
		if !ok || len(d.Children) == 0 {
			continue
		}
		childVal := yamlnode.NodeAtPath(valueNode, n.yamlPath)
		if childVal == nil {
			continue
		}
		unknowns := collectUnknownNodes(childVal, d.Children)
		if len(unknowns) == 0 {
			continue
		}
		end := i
		for end+1 < len(nodes) && nodes[end+1].depth > n.depth {
			end++
		}
		rows := make([]treeNode, len(unknowns))
		for j, u := range unknowns {
			u.depth = n.depth + 1
			u.yamlPath = append(append([]string{}, n.yamlPath...), u.yamlPath...)
			rows[j] = u
		}
		insertions = append(insertions, insertion{after: end, rows: rows})
	}
	if len(insertions) == 0 {
		return nodes
	}
	// Emit by insertion point; when a nested parent's run ends on the same row
	// as its ancestor's, the deeper unknowns go first so each row still sits
	// inside its own parent's subtree.
	sort.SliceStable(insertions, func(a, b int) bool {
		if insertions[a].after != insertions[b].after {
			return insertions[a].after < insertions[b].after
		}
		return insertions[a].rows[0].depth > insertions[b].rows[0].depth
	})
	result := make([]treeNode, 0, len(nodes)+len(insertions))
	k := 0
	for i, n := range nodes {
		result = append(result, n)
		for k < len(insertions) && insertions[k].after == i {
			result = append(result, insertions[k].rows...)
			k++
		}
	}
	return result
}

// defAtPath walks defs following path segments through Children and returns
// the FieldDef reached at the end of the path.
func defAtPath(defs []schema.FieldDef, path []string) (schema.FieldDef, bool) {
	var found schema.FieldDef
	cur := defs
	for i, seg := range path {
		ok := false
		for _, d := range cur {
			if d.YAMLName == seg {
				found, ok = d, true
				break
			}
		}
		if !ok {
			return schema.FieldDef{}, false
		}
		if i < len(path)-1 {
			cur = found.Children
		}
	}
	return found, true
}

func collectUnknownNodes(valueNode *yaml.Node, defs []schema.FieldDef) []treeNode {
	if valueNode == nil || valueNode.Kind != yaml.MappingNode {
		return nil
	}
	known := make(map[string]bool, len(defs))
	for _, d := range defs {
		known[d.YAMLName] = true
	}
	var out []treeNode
	for i := 0; i+1 < len(valueNode.Content); i += 2 {
		key := valueNode.Content[i].Value
		if !known[key] {
			out = append(out, treeNode{
				kind:     treeNodeUnknown,
				yamlPath: []string{key},
				label:    key,
				depth:    0,
				isLeaf:   true,
			})
		}
	}
	return out
}

// toggleNodeField adds or removes a single leaf field within valueNode (the
// block's value mapping). It clones valueNode before any mutation and returns
// the (possibly new) node; the caller must assign the result back. Returns
// valueNode unchanged when the toggle produces no structural change.
func toggleNodeField(valueNode *yaml.Node, ctx toggleCtx, node treeNode, checked bool) *yaml.Node {
	// Rows without a path (add-new, separators) carry nothing to toggle. The UI
	// never targets them, but a replayed action log (replayBlock) can.
	if len(node.yamlPath) == 0 {
		return valueNode
	}
	cloned := yamlnode.CloneNode(valueNode)
	if cloned.Kind != yaml.MappingNode {
		cloned.Kind = yaml.MappingNode
		cloned.Tag = ""
		cloned.Value = ""
		cloned.Content = nil
	}
	path := node.yamlPath
	if !applyToggleAt(cloned, path[:len(path)-1], path[len(path)-1], checked, ctx) {
		return valueNode
	}
	pruneEmptyMappings(cloned)
	reorderNestedMappingKeys(cloned, ctx.childDefs)
	return cloned
}

// findDuplicateMappingKey walks a value node depth-first and returns the
// dotted path of the first mapping key that appears more than once within the
// same mapping. schema.UnknownKeys cannot detect duplicates (yaml.v3 keeps the
// last value when decoding), so commit uses this as the final gate against
// persisting a corrupt mapping.
func findDuplicateMappingKey(n *yaml.Node) (string, bool) {
	return findDupKeyAt(n, nil)
}

func findDupKeyAt(n *yaml.Node, path []string) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Kind {
	case yaml.MappingNode:
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if seen[key] {
				return strings.Join(append(append([]string{}, path...), key), "."), true
			}
			seen[key] = true
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			child := append(append([]string{}, path...), n.Content[i].Value)
			if p, ok := findDupKeyAt(n.Content[i+1], child); ok {
				return p, true
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, c := range n.Content {
			if p, ok := findDupKeyAt(c, path); ok {
				return p, true
			}
		}
	}
	return "", false
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
