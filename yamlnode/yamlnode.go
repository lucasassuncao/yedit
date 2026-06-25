// Package yamlnode provides read-mostly query and navigation helpers over
// gopkg.in/yaml.v3 node trees, shared by the editor's editing flow and its
// validators. All functions treat nil or wrong-kind nodes as "absent" rather
// than panicking.
package yamlnode

import "gopkg.in/yaml.v3"

// ChildByKey returns the value node mapped to key in a MappingNode, or nil.
func ChildByKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// NodeAtPath navigates node following string segs and returns the terminal
// node. Returns nil when the path does not exist.
func NodeAtPath(node *yaml.Node, segs []string) *yaml.Node {
	for _, s := range segs {
		node = ChildByKey(node, s)
	}
	return node
}

// ScalarAt navigates node following segs and returns the scalar value at the
// terminal node. Returns "" when the path does not exist or the terminal node
// is not a scalar.
func ScalarAt(node *yaml.Node, segs []string) string {
	n := NodeAtPath(node, segs)
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// ScalarChild returns the scalar value of mapping node's direct key, or ""
// when the key is absent or its value is not a scalar.
func ScalarChild(node *yaml.Node, key string) string {
	c := ChildByKey(node, key)
	if c == nil || c.Kind != yaml.ScalarNode {
		return ""
	}
	return c.Value
}

// PresentNonEmpty reports whether node exists and is not an empty/null scalar.
// Mappings and sequences count as present even when empty.
func PresentNonEmpty(node *yaml.Node) bool {
	return node != nil && (node.Kind != yaml.ScalarNode || node.Value != "")
}

// RootMapping unmarshals raw and returns its root node. An empty document
// yields an empty mapping (so unconditional checks still run); invalid YAML
// yields ok=false.
func RootMapping(raw []byte) (*yaml.Node, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, false
	}
	if len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode}, true
	}
	return doc.Content[0], true
}

// JoinPath joins a dot-separated prefix with a key, omitting the dot when the
// prefix is empty.
func JoinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// CloneNode returns a deep copy of n so a snapshot can be mutated
// independently of the live tree. Returns nil for a nil input.
func CloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	cp := *n
	if n.Content != nil {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			cp.Content[i] = CloneNode(c)
		}
	}
	if n.Alias != nil {
		cp.Alias = CloneNode(n.Alias)
	}
	return &cp
}
