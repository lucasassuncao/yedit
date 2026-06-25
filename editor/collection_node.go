package editor

import (
	"fmt"

	"github.com/lucasassuncao/yedit/yamlnode"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// This file holds the node-based collection navigator: the structural
// counterpart of the seqEntry/collectionBuffer text layer. The collection's
// value node (be.node - a SequenceNode for []Struct, a MappingNode for
// map[string]Struct) is the single source of truth; entry labels, the entry
// list, and per-entry checkmarks are all derived from it.

// collValueNode parses raw ("key:\n  ...") into the collection's value node,
// coercing to an empty sequence (or mapping) when absent, empty, or the wrong
// kind so a fresh collection always has a writable node of the right shape.
func collValueNode(raw string, isMap bool) *yaml.Node {
	want := yaml.SequenceNode
	if isMap {
		want = yaml.MappingNode
	}
	if v := valueNodeOfSnippet(raw); v != nil && v.Kind == want {
		return v
	}
	return &yaml.Node{Kind: want}
}

// entryCount returns the number of entries in a collection value node.
func entryCount(node *yaml.Node, isMap bool) int {
	if node == nil {
		return 0
	}
	if isMap {
		return len(node.Content) / 2
	}
	return len(node.Content)
}

// entryValueNode returns the struct mapping of entry i (the value under a
// sequence item or map key), or nil when out of range.
func entryValueNode(node *yaml.Node, isMap bool, i int) *yaml.Node {
	if node == nil || i < 0 {
		return nil
	}
	if isMap {
		if 2*i+1 >= len(node.Content) {
			return nil
		}
		return node.Content[2*i+1]
	}
	if i >= len(node.Content) {
		return nil
	}
	return node.Content[i]
}

// entryLabel returns entry i's display label: the map key, or a sequence item's
// "name" field, falling back to "item N".
func entryLabel(node *yaml.Node, isMap bool, i int) string {
	if isMap {
		if 2*i < len(node.Content) {
			return node.Content[2*i].Value
		}
		return ""
	}
	if item := entryValueNode(node, false, i); item != nil {
		if n := yamlnode.ChildByKey(item, "name"); n != nil && n.Value != "" {
			return n.Value
		}
	}
	return fmt.Sprintf("item %d", i+1)
}

// entryViewYAML renders the single-entry editor text for entry i: "key:\n  - …"
// for sequences, "key:\n  <entryKey>:\n    …" for maps. The entry node is cloned
// before encoding so rendering never mutates the canonical tree's style.
func entryViewYAML(node *yaml.Node, key string, isMap bool, i int) string {
	if i < 0 || i >= entryCount(node, isMap) {
		return key + ":\n"
	}
	if isMap {
		wrap := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			yamlnode.CloneNode(node.Content[2*i]), yamlnode.CloneNode(node.Content[2*i+1]),
		}}
		return nodeToContent(key, wrap)
	}
	wrap := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{yamlnode.CloneNode(node.Content[i])}}
	return nodeToContent(key, wrap)
}

// viewHasMultipleSeqItems reports whether the YAML text contains more than one
// sequence item under the collection key. Used to catch the case where a user
// manually adds a second "- …" block to the single-entry editor - that extra
// entry would be silently dropped by parseEntryFromView, so we reject it early.
func viewHasMultipleSeqItems(view string) bool {
	blockVal := valueNodeOfSnippet(view)
	return blockVal != nil && blockVal.Kind == yaml.SequenceNode && len(blockVal.Content) > 1
}

// parseEntryFromView parses single-entry editor text back into the entry's key
// node (maps only) and value mapping. ok is false on a parse error or a shape
// that does not match the collection kind - the parse gate that keeps invalid
// text from corrupting the canonical node.
func parseEntryFromView(view string, isMap bool) (keyNode, valNode *yaml.Node, ok bool) {
	blockVal := valueNodeOfSnippet(view)
	if blockVal == nil {
		return nil, nil, false
	}
	if isMap {
		if blockVal.Kind != yaml.MappingNode || len(blockVal.Content) < 2 {
			return nil, nil, false
		}
		// Reject views that contain more than one map entry — the extra pairs
		// would be silently dropped by the two-node splice, corrupting data.
		if len(blockVal.Content) > 2 {
			return nil, nil, false
		}
		return blockVal.Content[0], blockVal.Content[1], true
	}
	if blockVal.Kind != yaml.SequenceNode || len(blockVal.Content) == 0 {
		return nil, nil, false
	}
	item := blockVal.Content[0]
	if item.Kind != yaml.MappingNode {
		return nil, nil, false
	}
	return nil, item, true
}

// setEntry splices a parsed key/value back into the collection node at index i.
func setEntry(node *yaml.Node, isMap bool, i int, keyNode, valNode *yaml.Node) {
	if isMap {
		if 2*i+1 < len(node.Content) {
			node.Content[2*i] = keyNode
			node.Content[2*i+1] = valNode
		}
		return
	}
	if i >= 0 && i < len(node.Content) {
		node.Content[i] = valNode
	}
}

// removeEntry splices entry i out of the collection node.
func removeEntry(node *yaml.Node, isMap bool, i int) {
	if isMap {
		if 2*i+1 < len(node.Content) {
			node.Content = append(node.Content[:2*i], node.Content[2*i+2:]...)
		}
		return
	}
	if i >= 0 && i < len(node.Content) {
		node.Content = append(node.Content[:i], node.Content[i+1:]...)
	}
}

// buildCollectionNodesFromNode builds the tree nodes for a collection from its
// value node: one seqItem per element (collapsed) with its child field nodes,
// checked states derived structurally, then the "+ add new" row.
func buildCollectionNodesFromNode(childDefs []schema.FieldDef, node *yaml.Node, isMap bool) []treeNode {
	var nodes []treeNode
	n := entryCount(node, isMap)
	if !isMap && n == 0 {
		nodes = append(nodes, treeNode{kind: treeNodeSeparator, label: "(empty list)", depth: 0, isLeaf: true})
	}
	for i := 0; i < n; i++ {
		label := entryLabel(node, isMap, i)
		nodes = append(nodes, treeNode{
			kind: treeNodeSeqItem, yamlPath: []string{label}, label: label,
			depth: 0, isLeaf: false, checked: true, seqIdx: i,
		})
		children := flattenDefsAsTree(childDefs, []string{label}, 1)
		children = deriveChecked(entryValueNode(node, isMap, i), children, true)
		nodes = append(nodes, children...)
	}
	nodes = append(nodes, treeNode{kind: treeNodeAddNew, label: "+ add new", depth: 0, isLeaf: true})
	return nodes
}
