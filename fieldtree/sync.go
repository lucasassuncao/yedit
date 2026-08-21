package fieldtree

import (
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// Structural sync helpers: they derive tree state from a *yaml.Node instead of
// re-parsing the YAML text on every edit. The tree is a pure projection of the
// node, so the two cannot disagree even while the text buffer is mid-edit.

// DeriveChecked recomputes the checked flag of every field node from valueNode,
// the block's value mapping.
//
// skipFirstSeg drops YAMLPath[0] for nodes below depth 0, for collection entries
// where path[0] is the entry label rather than a real mapping key. The returned
// slice is a copy; the input is left untouched.
func DeriveChecked(valueNode *yaml.Node, nodes []Node, skipFirstSeg bool) []Node {
	out := make([]Node, len(nodes))
	copy(out, nodes)
	for i, n := range out {
		// Leaves and inline-struct parents track key presence; openable fields
		// track non-empty content.
		if n.Kind != KindField {
			continue
		}
		path := n.YAMLPath
		start := 0
		if skipFirstSeg && n.Depth > 0 {
			start = 1
		}
		if len(path) <= start {
			out[i].Checked = false
			continue
		}
		cur := valueNode
		for j := start; j < len(path)-1 && cur != nil; j++ {
			cur = yamlnode.ChildByKey(cur, path[j])
		}
		if cur == nil {
			out[i].Checked = false
			continue
		}
		child := yamlnode.ChildByKey(cur, path[len(path)-1])
		if n.Openable || !n.IsLeaf {
			// These require real content, not just key presence, so {} is unchecked.
			out[i].Checked = child != nil && yamledit.NodeHasContent(child)
		} else {
			out[i].Checked = child != nil
			// A present-but-empty leaf is unfilled scaffolding that yamledit.PruneEmptyContent
			// strips at save, so flag it to render as a draft, not a committed field.
			out[i].EmptyValue = child != nil && !yamledit.NodeHasContent(child)
		}
	}
	return out
}

// SyncCheckedFromNode re-derives every field node's checked state from
// valueNode, then re-applies ADDED/AVAILABLE/UNKNOWN sectioning for struct trees
// and restores the cursor.
func SyncCheckedFromNode(tm Model, valueNode *yaml.Node) Model {
	var selectedPath []string
	if ni := tm.CurrentNodeIdx(); ni >= 0 && tm.Nodes[ni].Kind == KindField {
		selectedPath = tm.Nodes[ni].YAMLPath
	}

	tm.Nodes = DeriveChecked(valueNode, tm.Nodes, false)

	if !tm.isSeq {
		tm.Nodes = applySections(tm.Nodes, collectUnknownNodes(valueNode, tm.defs))
		tm.Nodes = injectNestedUnknowns(tm.Nodes, valueNode, tm.defs)
		tm = tm.restoreCursorToPath(selectedPath)
	}
	return tm
}

// injectNestedUnknowns inserts KindUnknown rows after an inline struct
// parent's descendant run whenever its value mapping holds keys the schema does
// not declare. applySections strips every KindUnknown before calling this
// again, keeping it idempotent.
func injectNestedUnknowns(nodes []Node, valueNode *yaml.Node, defs []schema.FieldDef) []Node {
	if valueNode == nil || valueNode.Kind != yaml.MappingNode || len(defs) == 0 {
		return nodes
	}
	// insertion queues unknown rows to emit right after nodes[after], past the
	// parent's contiguous run of descendants.
	type insertion struct {
		after int
		rows  []Node
	}
	var insertions []insertion
	for i, n := range nodes {
		if n.Kind != KindField || n.IsLeaf || n.Openable || len(n.YAMLPath) == 0 {
			continue
		}
		d, ok := defAtPath(defs, n.YAMLPath)
		if !ok || len(d.Children) == 0 {
			continue
		}
		childVal := yamlnode.NodeAtPath(valueNode, n.YAMLPath)
		if childVal == nil {
			continue
		}
		unknowns := collectUnknownNodes(childVal, d.Children)
		if len(unknowns) == 0 {
			continue
		}
		end := i
		for end+1 < len(nodes) && nodes[end+1].Depth > n.Depth {
			end++
		}
		rows := make([]Node, len(unknowns))
		for j, u := range unknowns {
			u.Depth = n.Depth + 1
			u.YAMLPath = append(append([]string{}, n.YAMLPath...), u.YAMLPath...)
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
		return insertions[a].rows[0].Depth > insertions[b].rows[0].Depth
	})
	result := make([]Node, 0, len(nodes)+len(insertions))
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

func collectUnknownNodes(valueNode *yaml.Node, defs []schema.FieldDef) []Node {
	if valueNode == nil || valueNode.Kind != yaml.MappingNode {
		return nil
	}
	known := make(map[string]bool, len(defs))
	for _, d := range defs {
		known[d.YAMLName] = true
	}
	var out []Node
	for i := 0; i+1 < len(valueNode.Content); i += 2 {
		key := valueNode.Content[i].Value
		if !known[key] {
			out = append(out, Node{
				Kind:     KindUnknown,
				YAMLPath: []string{key},
				Label:    key,
				Depth:    0,
				IsLeaf:   true,
			})
		}
	}
	return out
}

// ToggleNodeField adds or removes a single leaf field within valueNode. It
// clones before mutating and returns the new node, so the caller must assign the
// result back; a toggle with no structural effect returns valueNode unchanged.
func ToggleNodeField(valueNode *yaml.Node, ctx yamledit.ToggleCtx, node Node, checked bool) *yaml.Node {
	// Rows without a path carry nothing to toggle. The UI never targets them, but
	// a replayed action log can.
	if len(node.YAMLPath) == 0 {
		return valueNode
	}
	cloned := yamlnode.CloneNode(valueNode)
	if cloned.Kind != yaml.MappingNode {
		cloned.Kind = yaml.MappingNode
		cloned.Tag = ""
		cloned.Value = ""
		cloned.Content = nil
	}
	path := node.YAMLPath
	if !yamledit.ApplyToggleAt(cloned, path[:len(path)-1], path[len(path)-1], checked, ctx) {
		return valueNode
	}
	yamledit.PruneEmptyMappings(cloned)
	yamledit.ReorderNestedMappingKeys(cloned, ctx.ChildDefs)
	return cloned
}

// CollectionNodes builds a collection's tree Nodes: one collapsed
// seqItem per element with its child field nodes and derived checked states,
// then the "+ add new" row.
func CollectionNodes(childDefs []schema.FieldDef, node *yaml.Node, isMap bool) []Node {
	var nodes []Node
	n := yamledit.EntryCount(node, isMap)
	if !isMap && n == 0 {
		nodes = append(nodes, Node{Kind: KindSeparator, Label: "(empty list)", Depth: 0, IsLeaf: true})
	}
	for i := 0; i < n; i++ {
		label := yamledit.EntryLabel(node, isMap, i)
		nodes = append(nodes, Node{
			Kind: KindSeqItem, YAMLPath: []string{label}, Label: label,
			Depth: 0, IsLeaf: false, Checked: true, SeqIdx: i,
		})
		children := flattenDefsAsTree(childDefs, []string{label}, 1)
		children = DeriveChecked(yamledit.EntryValueNode(node, isMap, i), children, true)
		nodes = append(nodes, children...)
	}
	nodes = append(nodes, Node{Kind: KindAddNew, Label: "+ add new", Depth: 0, IsLeaf: true})
	return nodes
}
