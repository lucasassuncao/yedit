package yamlnode

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Navigate traverses node following segs, expanding sequences and
// dict-of-structs automatically. onArrival is called with the reached node and
// its expanded dot/index path when segs is exhausted. A nil node is treated as
// absent: no calls are made.
func Navigate(node *yaml.Node, segs []string, path string, onArrival func(node *yaml.Node, path string)) {
	navigate(node, segs, path, false, onArrival)
}

// navigate implements Navigate. matched tracks whether any segment has been
// consumed yet: the dict-of-structs fallback is disabled at the root so a
// missing top-level key is "absent" rather than a document-wide search,
// mirroring walkLeaf.
func navigate(node *yaml.Node, segs []string, path string, matched bool, onArrival func(node *yaml.Node, path string)) {
	node = resolveAlias(node)
	if node == nil {
		return
	}
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			navigate(item, segs, fmt.Sprintf("%s[%d]", path, i), matched, onArrival)
		}
		return
	}
	if len(segs) == 0 {
		onArrival(node, path)
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}
	next, rest := segs[0], segs[1:]
	if child := ChildByKey(node, next); child != nil {
		navigate(child, rest, JoinPath(path, next), true, onArrival)
		return
	}
	if !matched {
		return
	}
	// Key not found at this level - treat as a dict-of-structs: check all values.
	for i := 0; i+1 < len(node.Content); i += 2 {
		navigate(node.Content[i+1], segs, JoinPath(path, node.Content[i].Value), matched, onArrival)
	}
}

// ForEachLeaf calls fn with every node reached by the dotted path and its full
// expanded path. Sequences are expanded at every level, and - once at least
// one segment has matched - a missing segment falls back to dict-of-structs
// descent (every mapping value is searched), mirroring Navigate. The leaf node
// is delivered as-is (scalar, sequence, or mapping); fn never receives nil -
// absent paths simply produce no calls.
func ForEachLeaf(root *yaml.Node, path string, fn func(node *yaml.Node, where string)) {
	walkLeaf(root, strings.Split(path, "."), "", false, fn)
}

// walkLeaf implements ForEachLeaf. matched tracks whether any segment has been
// consumed yet: the dict-of-structs fallback is disabled at the root so a
// missing top-level key is "absent" rather than a document-wide search.
func walkLeaf(node *yaml.Node, segs []string, path string, matched bool, fn func(node *yaml.Node, where string)) {
	node = resolveAlias(node)
	if node == nil {
		return
	}
	if node.Kind == yaml.SequenceNode {
		for i, item := range node.Content {
			walkLeaf(item, segs, fmt.Sprintf("%s[%d]", path, i), matched, fn)
		}
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}
	key, rest := segs[0], segs[1:]
	if child := ChildByKey(node, key); child != nil {
		childPath := JoinPath(path, key)
		if len(rest) == 0 {
			fn(child, childPath)
			return
		}
		walkLeaf(child, rest, childPath, true, fn)
		return
	}
	if !matched {
		return
	}
	// Key not found at this level - treat as a dict-of-structs: search all values.
	for i := 0; i+1 < len(node.Content); i += 2 {
		walkLeaf(node.Content[i+1], segs, JoinPath(path, node.Content[i].Value), matched, fn)
	}
}
