package yamledit

import (
	"fmt"

	"github.com/lucasassuncao/yedit/yamlnode"

	"gopkg.in/yaml.v3"
)

// The node-based collection navigator. be.node - a SequenceNode for []Struct, a
// MappingNode for map[string]Struct - is the single source of truth; entry
// labels, the entry list, and per-entry checkmarks are all derived from it.

// CollValueNode parses raw ("key:\n  ...") into the collection's value node,
// coercing anything absent or of the wrong kind to an empty node of the right
// shape, so a fresh collection is always writable.
func CollValueNode(raw string, isMap bool) *yaml.Node {
	want := yaml.SequenceNode
	if isMap {
		want = yaml.MappingNode
	}
	if v := ValueNodeOfSnippet(raw); v != nil && v.Kind == want {
		return v
	}
	return &yaml.Node{Kind: want}
}

// EntryCount returns the number of entries in a collection value node.
func EntryCount(node *yaml.Node, isMap bool) int {
	if node == nil {
		return 0
	}
	if isMap {
		return len(node.Content) / 2
	}
	return len(node.Content)
}

// EntryValueNode returns the struct mapping of entry i (the value under a
// sequence item or map key), or nil when out of range.
func EntryValueNode(node *yaml.Node, isMap bool, i int) *yaml.Node {
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

// EntryLabel returns entry i's display label: the map key, or a sequence item's
// "name" field, falling back to "item N".
func EntryLabel(node *yaml.Node, isMap bool, i int) string {
	if isMap {
		if i >= 0 && 2*i < len(node.Content) {
			return node.Content[2*i].Value
		}
		return ""
	}
	if item := EntryValueNode(node, false, i); item != nil {
		if n := yamlnode.ChildByKey(item, "name"); n != nil && n.Value != "" {
			return n.Value
		}
	}
	return fmt.Sprintf("item %d", i+1)
}

// EntryViewYAML renders the single-entry editor text for entry i: "key:\n  - …"
// for sequences, "key:\n  <entryKey>:\n    …" for maps. The node is cloned first
// so rendering never mutates the canonical tree's style.
func EntryViewYAML(node *yaml.Node, key string, isMap bool, i int) string {
	if i < 0 || i >= EntryCount(node, isMap) {
		return key + ":\n"
	}
	if isMap {
		wrap := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			yamlnode.CloneNode(node.Content[2*i]), yamlnode.CloneNode(node.Content[2*i+1]),
		}}
		return NodeToContent(key, wrap)
	}
	wrap := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{yamlnode.CloneNode(node.Content[i])}}
	return NodeToContent(key, wrap)
}

// ViewHasMultipleSeqItems catches a second "- …" block added by hand to the
// single-entry editor: ParseEntryFromView would drop it, so reject it early.
func ViewHasMultipleSeqItems(view string) bool {
	blockVal := ValueNodeOfSnippet(view)
	return blockVal != nil && blockVal.Kind == yaml.SequenceNode && len(blockVal.Content) > 1
}

// ParseEntryFromView parses single-entry editor text back into the entry's key
// node (maps only) and value mapping. ok is false on a parse error or a shape
// mismatch: this is the gate that keeps invalid text out of the canonical node.
func ParseEntryFromView(view string, isMap bool) (keyNode, valNode *yaml.Node, ok bool) {
	blockVal := ValueNodeOfSnippet(view)
	if blockVal == nil {
		return nil, nil, false
	}
	if isMap {
		if blockVal.Kind != yaml.MappingNode || len(blockVal.Content) < 2 {
			return nil, nil, false
		}
		// More than one map entry: the two-node splice would drop the extra pairs.
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

// SetEntry splices a parsed key/value back into the collection node at index i.
func SetEntry(node *yaml.Node, isMap bool, i int, keyNode, valNode *yaml.Node) {
	if isMap {
		if i >= 0 && 2*i+1 < len(node.Content) {
			node.Content[2*i] = keyNode
			node.Content[2*i+1] = valNode
		}
		return
	}
	if i >= 0 && i < len(node.Content) {
		node.Content[i] = valNode
	}
}

// RemoveEntry splices entry i out of the collection node.
func RemoveEntry(node *yaml.Node, isMap bool, i int) {
	if isMap {
		if i >= 0 && 2*i+1 < len(node.Content) {
			node.Content = append(node.Content[:2*i], node.Content[2*i+2:]...)
		}
		return
	}
	if i >= 0 && i < len(node.Content) {
		node.Content = append(node.Content[:i], node.Content[i+1:]...)
	}
}
