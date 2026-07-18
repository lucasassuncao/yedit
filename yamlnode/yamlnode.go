// Package yamlnode provides read-mostly query and navigation helpers over
// gopkg.in/yaml.v3 node trees, shared by the editor's editing flow and its
// validators. All functions treat nil or wrong-kind nodes as "absent" rather
// than panicking.
package yamlnode

import "gopkg.in/yaml.v3"

// ChildByKey returns the value node mapped to key in a MappingNode, or nil.
// Alias values are resolved to their anchor targets, and when the direct key
// is absent the mappings merged in via a "<<" merge key are searched.
func ChildByKey(mapping *yaml.Node, key string) *yaml.Node {
	return childByKey(mapping, key, nil)
}

// childByKey implements ChildByKey. seen guards against merge cycles in
// hand-built trees (the parser cannot produce them); it is allocated lazily
// on the first merge-key descent.
func childByKey(mapping *yaml.Node, key string, seen map[*yaml.Node]bool) *yaml.Node {
	mapping = resolveAlias(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode || seen[mapping] {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return resolveAlias(mapping.Content[i+1])
		}
	}
	// Direct key absent: look through the mappings merged in via "<<".
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		k := mapping.Content[i]
		if k.Tag != "!!merge" && k.Value != "<<" {
			continue
		}
		if seen == nil {
			seen = make(map[*yaml.Node]bool)
		}
		seen[mapping] = true
		v := resolveAlias(mapping.Content[i+1])
		if v == nil {
			continue
		}
		sources := []*yaml.Node{v}
		if v.Kind == yaml.SequenceNode {
			sources = v.Content
		}
		for _, src := range sources {
			if c := childByKey(src, key, seen); c != nil {
				return c
			}
		}
	}
	return nil
}

// resolveAlias follows alias links to the anchored node, guarding against
// cycles in hand-built trees. Non-alias nodes are returned as-is; nil stays
// nil, and a broken or cyclic alias resolves to nil ("absent").
func resolveAlias(n *yaml.Node) *yaml.Node {
	var seen map[*yaml.Node]bool
	for n != nil && n.Kind == yaml.AliasNode {
		if seen[n] {
			return nil
		}
		if seen == nil {
			seen = make(map[*yaml.Node]bool)
		}
		seen[n] = true
		n = n.Alias
	}
	return n
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
// independently of the live tree. Returns nil for a nil input. Alias nodes in
// the clone point at the cloned counterpart of their anchor target, preserving
// anchor/alias identity; a target outside the cloned subtree is deep-copied.
func CloneNode(n *yaml.Node) *yaml.Node {
	clones := make(map[*yaml.Node]*yaml.Node)
	cp := cloneTree(n, clones)
	// Re-point each cloned alias at the cloned counterpart of its target. The
	// pairs are collected first because an out-of-tree target grows the map.
	type pair struct{ orig, clone *yaml.Node }
	var fixups []pair
	for orig, c := range clones {
		if orig.Alias != nil {
			fixups = append(fixups, pair{orig, c})
		}
	}
	for _, p := range fixups {
		p.clone.Alias = cloneTree(p.orig.Alias, clones)
	}
	return cp
}

// cloneTree deep-copies n's content, recording every original->clone pair in
// clones. Alias pointers are left for CloneNode to re-point after the walk.
func cloneTree(n *yaml.Node, clones map[*yaml.Node]*yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if c, ok := clones[n]; ok {
		return c
	}
	cp := *n
	clones[n] = &cp
	if n.Content != nil {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			cp.Content[i] = cloneTree(c, clones)
		}
	}
	return &cp
}
