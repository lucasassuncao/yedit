package editor

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// Structural sync helpers: they derive tree state from a *yaml.Node instead of
// re-parsing the YAML text on every edit. The tree is a pure projection of the
// node, so the two cannot disagree even while the text buffer is mid-edit.

// deriveChecked recomputes the checked flag of every field node from valueNode,
// the block's value mapping.
//
// skipFirstSeg drops yamlPath[0] for nodes below depth 0, for collection entries
// where path[0] is the entry label rather than a real mapping key. The returned
// slice is a copy; the input is left untouched.
func deriveChecked(valueNode *yaml.Node, nodes []treeNode, skipFirstSeg bool) []treeNode {
	out := make([]treeNode, len(nodes))
	copy(out, nodes)
	for i, n := range out {
		// Leaves and inline-struct parents track key presence; openable fields
		// track non-empty content.
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
			// These require real content, not just key presence, so {} is unchecked.
			out[i].checked = child != nil && nodeHasContent(child)
		} else {
			out[i].checked = child != nil
			// A present-but-empty leaf is unfilled scaffolding that pruneEmptyContent
			// strips at save, so flag it to render as a draft, not a committed field.
			out[i].emptyValue = child != nil && !nodeHasContent(child)
		}
	}
	return out
}

// syncTreeCheckedFromNode re-derives every field node's checked state from
// valueNode, then re-applies ADDED/AVAILABLE/UNKNOWN sectioning for struct trees
// and restores the cursor.
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

// injectNestedUnknowns inserts treeNodeUnknown rows after an inline struct
// parent's descendant run whenever its value mapping holds keys the schema does
// not declare. applySections strips every treeNodeUnknown before calling this
// again, keeping it idempotent.
func injectNestedUnknowns(nodes []treeNode, valueNode *yaml.Node, defs []schema.FieldDef) []treeNode {
	if valueNode == nil || valueNode.Kind != yaml.MappingNode || len(defs) == 0 {
		return nodes
	}
	// insertion queues unknown rows to emit right after nodes[after], past the
	// parent's contiguous run of descendants.
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
	// When a nested parent's run ends on the same row as its ancestor's, the
	// deeper unknowns go first so each row stays inside its own parent's subtree.
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

// defAtPath walks defs through Children and returns the FieldDef at the end of
// path.
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

// toggleNodeField adds or removes a single leaf field within valueNode. It
// clones before mutating and returns the new node, so the caller must assign the
// result back; a toggle with no structural effect returns valueNode unchanged.
func toggleNodeField(valueNode *yaml.Node, ctx toggleCtx, node treeNode, checked bool) *yaml.Node {
	// Rows without a path carry nothing to toggle. The UI never targets them, but
	// a replayed action log can.
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

// findDuplicateMappingKey returns the dotted path of the first key repeated
// within one mapping. schema.UnknownKeys cannot see duplicates (yaml.v3 keeps
// the last value), so commit uses this as its final gate.
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

// nodeHasContent reports whether a value node carries real content: a null
// scalar, empty string, or empty list/map counts as empty.
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

// blockValueNode parses "<key>:\n  ..." into the value node mapped to key, the
// canonical node for a block editor. Empty or unparseable content yields an
// empty mapping, so a fresh block always has a writable node.
func blockValueNode(content string) *yaml.Node {
	if v := valueNodeOfSnippet(content); v != nil {
		return v
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}

// blockValueNodeOrNil is blockValueNode but returns nil when non-empty content
// fails to parse, so callers can tell "empty block" from "corrupt content" and
// surface an error instead of masking it.
func blockValueNodeOrNil(content string) *yaml.Node {
	if strings.TrimSpace(content) == "" {
		return &yaml.Node{Kind: yaml.MappingNode}
	}
	return valueNodeOfSnippet(content)
}
